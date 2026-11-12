package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Every route in the API spec is a route of the server, with the same path,
// so clients that follow the spec don't get 404s (the spec once had /feed/
// for /feed)
func TestAPISpecMatchesTheRoutes(t *testing.T) {
	data, err := APIDocs.ReadFile("docs/swagger.json")
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if len(data) == 0 {
		t.Skip("docs/swagger.json is empty; make build generates it")
	}
	var spec struct {
		BasePath string `json:"basePath"`
		Info     struct {
			Description string `json:"description"`
		} `json:"info"`
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("decode spec: %v", err)
	}
	if spec.BasePath != "/" {
		t.Fatalf("basePath = %q, want /", spec.BasePath)
	}
	// A @description after a security definition swag doesn't describe
	// replaces this one
	if !strings.HasPrefix(spec.Info.Description, "Authenticate with an API key") {
		t.Errorf("the spec's description = %q, want how to authenticate", spec.Info.Description)
	}

	router := chi.NewRouter()
	router.Route("/api", apiRoutes)
	router.Route("/storage", storageRoutes(http.NotFoundHandler()))
	params := strings.NewReplacer("{id}", "1", "{name}", "deploy", "{path}", "task_1.log")
	for path, operations := range spec.Paths {
		for method := range operations {
			method = strings.ToUpper(method)
			if router.Find(chi.NewRouteContext(), method, params.Replace(path)) == "" {
				t.Errorf("%s %s is in the spec but isn't a route", method, path)
			}
		}
	}
}
