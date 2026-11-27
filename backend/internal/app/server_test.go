package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewStartsWithoutMockUsersOrJokes(t *testing.T) {
	server, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer server.Close()

	server.mu.RLock()
	defer server.mu.RUnlock()
	if got := len(server.users); got != 0 {
		t.Errorf("expected no initial users, got %d", got)
	}
	if got := len(server.jokes); got != 0 {
		t.Errorf("expected no initial jokes, got %d", got)
	}
	if got := len(server.clusters); got != 9 {
		t.Errorf("expected the 9 configured taxonomy clusters, got %d", got)
	}
}

func TestEmptyCollectionEndpointsReturnEmptyArrays(t *testing.T) {
	server, err := New(Config{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer server.Close()

	for _, test := range []struct {
		path string
		key  string
	}{
		{path: "/api/jokes?page=1&limit=50", key: "jokes"},
		{path: "/api/triggers", key: "triggers"},
		{path: "/api/search?q=unmatched&mode=text", key: "keyword_results"},
		{path: "/api/search?q=unmatched&mode=text", key: "semantic_results"},
		{path: "/api/search?q=unmatched&mode=text", key: "results"},
	} {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			var items []json.RawMessage
			if err := json.Unmarshal(payload[test.key], &items); err != nil {
				t.Fatalf("decode %s: %v", test.key, err)
			}
			if items == nil || len(items) != 0 {
				t.Errorf("%s = %v, want an empty array", test.key, items)
			}
		})
	}
}
