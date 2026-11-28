package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type searchTestRoundTripper func(*http.Request) (*http.Response, error)

func (roundTrip searchTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func newSearchTestServer(t *testing.T) *Server {
	t.Helper()
	server, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	t.Cleanup(server.Close)
	return server
}

func saunaJoke() Joke {
	joke := Joke{
		ID:       "1364",
		Text:     "Полина пошла с тремя мужиками в баню и вообще не парилась.",
		Triggers: []string{"париться", "баня", "парилка"},
		Cluster:  Cluster{ID: 1, Name: "Каламбуры и игра слов", Slug: "puns"},
		Author:   Author{ID: "1", Username: "Автор"},
	}
	joke.Embedding = embeddingBytes(hashEmbedding(embeddingText(joke.Text, joke.Triggers)))
	return joke
}

func successfulEmbeddingTransport(t *testing.T) searchTestRoundTripper {
	t.Helper()
	return func(request *http.Request) (*http.Response, error) {
		var payload struct {
			ModelURI string `json:"modelUri"`
			Text     string `json:"text"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			return nil, err
		}
		if !strings.Contains(payload.ModelURI, "text-search-") {
			t.Fatalf("unexpected embedding model URI %q", payload.ModelURI)
		}
		values := make([]float32, embeddingDimensions)
		values[0] = 1
		body, err := json.Marshal(struct {
			Embedding []float32 `json:"embedding"`
		}{Embedding: values})
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(body)),
			Request:    request,
		}, nil
	}
}

func configureSearchEmbeddings(server *Server, transport http.RoundTripper) {
	server.ai.YandexAPIKey = "test-key"
	server.ai.YandexFolderID = "test-folder"
	server.ai.HTTP = &http.Client{Transport: transport}
}

func TestSemanticEmbeddingBytesRejectsUnavailableOrInvalidVectors(t *testing.T) {
	values := hashEmbedding("fallback-only")
	if embedding := semanticEmbeddingBytes(values, errors.New("Yandex unavailable")); embedding != nil {
		t.Fatal("semanticEmbeddingBytes() persisted a fallback vector")
	}
	if embedding := semanticEmbeddingBytes(make([]float32, embeddingDimensions), nil); embedding != nil {
		t.Fatal("semanticEmbeddingBytes() persisted a zero vector")
	}
	values[0] = 1
	if embedding := semanticEmbeddingBytes(values, nil); len(embedding) != embeddingDimensions*4 {
		t.Fatalf("valid embedding length = %d, want %d", len(embedding), embeddingDimensions*4)
	}
}

func TestStoredSemanticVectorRejectsLegacyHashEmbedding(t *testing.T) {
	joke := saunaJoke()
	if _, ok := storedSemanticVector(joke); ok {
		t.Fatal("legacy hash embedding was accepted as a semantic vector")
	}
}

func TestSemanticModeFallsBackToTagsWhenQueryEmbeddingIsUnavailable(t *testing.T) {
	server := newSearchTestServer(t)
	joke := saunaJoke()
	server.cacheJoke(joke)

	if _, err := server.semanticSearch(context.Background(), "баня", nil, 24); err == nil {
		t.Fatal("semanticSearch() error = nil without an embeddings client configuration")
	}
	if server.hasSemanticVector(1364) {
		t.Fatal("fallback hash vector should not be used for semantic ranking")
	}

	response := server.search(context.Background(), "баня", "semantic")
	if !response.SemanticFallback {
		t.Fatal("SemanticFallback = false, want true when query embedding is unavailable")
	}
	if len(response.SemanticResults) == 0 || response.SemanticResults[0].ID != "1364" {
		t.Fatalf("semantic fallback results = %#v, want sauna joke 1364", response.SemanticResults)
	}
}

func TestSemanticModeIncludesLexicalMatchForMissingDocumentVector(t *testing.T) {
	server := newSearchTestServer(t)
	configureSearchEmbeddings(server, successfulEmbeddingTransport(t))
	server.reindexMu.Lock()
	server.reindexClosed = true // Keep this search test independent of the background repair worker.
	server.reindexMu.Unlock()

	server.cacheJoke(saunaJoke())
	response := server.search(context.Background(), "баня", "semantic")
	if !response.SemanticFallback {
		t.Fatal("SemanticFallback = false, want true when a document vector is missing")
	}
	if len(response.SemanticResults) == 0 || response.SemanticResults[0].ID != "1364" {
		t.Fatalf("semantic results = %#v, want sauna joke 1364", response.SemanticResults)
	}
}

func TestSemanticModePrioritizesDirectTriggerMatchAlongsideVectors(t *testing.T) {
	server := newSearchTestServer(t)
	configureSearchEmbeddings(server, successfulEmbeddingTransport(t))

	joke := saunaJoke()
	values := make([]float32, embeddingDimensions)
	values[1] = 1 // Orthogonal to the fake query embedding.
	joke.Embedding = embeddingBytes(values)
	server.cacheJoke(joke)

	response := server.search(context.Background(), "баня", "semantic")
	if response.SemanticFallback {
		t.Fatal("SemanticFallback = true, want normal vector search")
	}
	if len(response.SemanticResults) == 0 || response.SemanticResults[0].ID != "1364" {
		t.Fatalf("semantic results = %#v, want direct trigger match 1364 first", response.SemanticResults)
	}
	if response.SemanticResults[0].Relevance < .98 {
		t.Fatalf("direct trigger relevance = %v, want at least 0.98", response.SemanticResults[0].Relevance)
	}
}

func TestMissingLegacyEmbeddingIsBackfilled(t *testing.T) {
	server := newSearchTestServer(t)
	configureSearchEmbeddings(server, successfulEmbeddingTransport(t))
	server.cacheJoke(saunaJoke())

	server.reindexMissingEmbeddings()

	if !server.hasSemanticVector(1364) {
		t.Fatal("backfilled joke still has no usable semantic vector")
	}
	joke, ok := server.getJoke(context.Background(), 1364)
	if !ok {
		t.Fatal("backfilled joke disappeared from the cache")
	}
	if len(joke.Embedding) != embeddingDimensions*4 || isLegacyHashEmbedding(joke) {
		t.Fatal("backfilled joke does not contain a real document embedding")
	}
	if server.hasMissingSemanticVectors() {
		t.Fatal("missing semantic vectors remain after backfilling the only joke")
	}
}
