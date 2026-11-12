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

// Every route is in the API spec, so the docs can't fall behind the code.
// Only these are described elsewhere.
func TestEveryRouteIsDocumented(t *testing.T) {
	data, err := APIDocs.ReadFile("docs/swagger.json")
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if len(data) == 0 {
		t.Skip("docs/swagger.json is empty; make build generates it")
	}
	var spec struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("decode spec: %v", err)
	}
	undocumented := map[string]bool{
		// MCP is described by /docs/mcp.json and the help page
		"POST /mcp": true,
		// The docs and the agent guide themselves
		"GET /docs/api/": true, "GET /docs/swagger.json": true, "GET /docs/mcp.json": true, "GET /llms.txt": true,
		// Storage is documented as one GET; HEAD is the same
		"HEAD /storage/build/*": true,
	}
	documented := func(method, route string) bool {
		route = strings.ReplaceAll(route, "/*/", "/")
		if strings.HasPrefix(route, "/storage/build/") {
			route = "/storage/build/{id}/{path}"
		}
		for path, operations := range spec.Paths {
			if strings.TrimSuffix(path, "/") == strings.TrimSuffix(route, "/") {
				_, ok := operations[strings.ToLower(method)]
				return ok
			}
		}
		return false
	}
	routers := map[string]chi.Routes{"password": apiRouter().(chi.Routes)}
	keyRouter, ok := newAPIKeyRouter(http.NotFoundHandler()).(chi.Routes)
	if !ok {
		t.Fatal("the key router isn't a chi router")
	}
	routers["api key"] = keyRouter
	storage := chi.NewRouter()
	storage.Route("/storage", storageRoutes(http.NotFoundHandler()))
	routers["storage"] = storage
	for name, router := range routers {
		err := chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
			pattern := method + " " + strings.ReplaceAll(route, "/*/", "/")
			if !undocumented[pattern] && !documented(method, route) {
				t.Errorf("%s (%s router) isn't in the API spec", pattern, name)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s routes: %v", name, err)
		}
	}
}

func TestMCPDocs(t *testing.T) {
	docs, err := mcpToolDocs(t.Context())
	if err != nil {
		t.Fatalf("mcpToolDocs: %v", err)
	}
	byName := map[string]MCPToolDoc{}
	for _, doc := range docs {
		byName[doc.Name] = doc
	}
	if len(docs) != 8 {
		t.Errorf("documented %d tools, want 8", len(docs))
	}
	summary := byName["build_summary"]
	if summary.Scope != "read" || summary.Description == "" || len(summary.Inputs) != 3 || summary.Inputs[0].Name != "id" || !summary.Inputs[0].Required || summary.Inputs[0].Type != "integer" {
		t.Errorf("build_summary = %+v, want scope read and id first, required", summary)
	}
	if byName["run_job"].Scope != "run" || byName["whoami"].Scope != "any" {
		t.Errorf("scopes: run_job %q, whoami %q", byName["run_job"].Scope, byName["whoami"].Scope)
	}
	for _, doc := range docs {
		if doc.Scope == "" {
			t.Errorf("%s has no scope", doc.Name)
		}
		for _, input := range doc.Inputs {
			if input.Description == "" {
				t.Errorf("%s.%s has no description", doc.Name, input.Name)
			}
		}
	}
}
