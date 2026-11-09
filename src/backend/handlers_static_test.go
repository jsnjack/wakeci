package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// TestHandleVueResources checks what the embedded frontend serves. all:assets
// embeds hidden files too, so a hidden one (say Vite's .vite/manifest.json)
// must not be served, while a chunk starting with _ must
func TestHandleVueResources(t *testing.T) {
	files := fstest.MapFS{
		"assets/index.html":          {Data: []byte("index")},
		"assets/assets/app.js":       {Data: []byte("app")},
		"assets/assets/_shared.js":   {Data: []byte("shared")},
		"assets/.vite/manifest.json": {Data: []byte("{}")},
		"assets/assets/.hidden.js":   {Data: []byte("hidden")},
	}
	handler := HandleVueResources(http.FileServer(http.FS(files)))
	tests := []struct {
		path string
		want int
	}{
		{"/", http.StatusOK},
		{"/jobs", http.StatusOK},
		{"/assets/app.js", http.StatusOK},
		{"/assets/_shared.js", http.StatusOK},
		{"/.vite/manifest.json", http.StatusNotFound},
		{"/assets/.hidden.js", http.StatusNotFound},
		{"/assets/../.vite/manifest.json", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.want {
				t.Errorf("GET %s = %d, want %d", tt.path, rec.Code, tt.want)
			}
		})
	}
}
