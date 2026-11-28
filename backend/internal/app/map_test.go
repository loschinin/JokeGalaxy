package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type jokeListResponse struct {
	Jokes   []Joke `json:"jokes"`
	HasMore bool   `json:"has_more"`
}

func requestJokePage(t *testing.T, server *Server, path string) jokeListResponse {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d", path, response.Code, http.StatusOK)
	}
	var payload jokeListResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode GET %s response: %v", path, err)
	}
	return payload
}

func TestMapJokeListIncludesDuplicatesAndPaginatesThem(t *testing.T) {
	server, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer server.Close()

	server.cacheJoke(Joke{ID: "1", Text: "Обычная шутка"})
	server.cacheJoke(Joke{ID: "2", Text: "Шутка, помеченная дублем", IsDuplicate: true})

	publicPage := requestJokePage(t, server, "/api/jokes?page=1&limit=50")
	if len(publicPage.Jokes) != 1 || publicPage.Jokes[0].IsDuplicate {
		t.Fatalf("public joke list = %#v, want only the non-duplicate joke", publicPage.Jokes)
	}

	firstMapPage := requestJokePage(t, server, "/api/map/jokes?page=1&limit=1")
	if len(firstMapPage.Jokes) != 1 || !firstMapPage.HasMore {
		t.Fatalf("first map page = %#v, want one joke and has_more=true", firstMapPage)
	}
	secondMapPage := requestJokePage(t, server, "/api/map/jokes?page=2&limit=1")
	if len(secondMapPage.Jokes) != 1 || secondMapPage.HasMore {
		t.Fatalf("second map page = %#v, want one joke and has_more=false", secondMapPage)
	}

	foundDuplicate := false
	for _, joke := range append(firstMapPage.Jokes, secondMapPage.Jokes...) {
		if joke.ID == "2" && joke.IsDuplicate {
			foundDuplicate = true
		}
	}
	if !foundDuplicate {
		t.Fatal("paginated map results did not include the joke marked as a duplicate")
	}
}
