package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

// TestHandleVueResources checks what the embedded frontend serves. Vite's
// chunks under /assets/ and files at the top level are static files, with
// hidden ones refused since all:assets embeds them too; every other path is a
// page of the app, also when it holds a dot (a job named deploy.prod)
func TestHandleVueResources(t *testing.T) {
	files := fstest.MapFS{
		"assets/index.html":          {Data: []byte("index")},
		"assets/favicon.ico":         {Data: []byte("icon")},
		"assets/.env":                {Data: []byte("hidden")},
		"assets/assets/app.js":       {Data: []byte("app")},
		"assets/assets/_shared.js":   {Data: []byte("shared")},
		"assets/assets/.hidden.js":   {Data: []byte("hidden")},
		"assets/.vite/manifest.json": {Data: []byte("manifest")},
	}
	handler := HandleVueResources(http.FileServer(http.FS(files)))
	tests := []struct {
		path     string
		wantCode int
		// wantBody is checked when wantCode is 200
		wantBody string
	}{
		{"/", http.StatusOK, "index"},
		{"/jobs", http.StatusOK, "index"},
		{"/job/deploy.prod", http.StatusOK, "index"},
		{"/job/.env", http.StatusOK, "index"},
		{"/favicon.ico", http.StatusOK, "icon"},
		{"/assets/app.js", http.StatusOK, "app"},
		{"/assets/_shared.js", http.StatusOK, "shared"},
		{"/assets/missing.js", http.StatusNotFound, ""},
		{"/.env", http.StatusNotFound, ""},
		{"/assets/.hidden.js", http.StatusNotFound, ""},
		{"/assets/../.vite/manifest.json", http.StatusNotFound, ""},
		{"/.vite/manifest.json", http.StatusOK, "index"},
		// Not a directory listing of the build
		{"/assets/", http.StatusOK, "index"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.wantCode {
				t.Fatalf("GET %s = %d, want %d", tt.path, rec.Code, tt.wantCode)
			}
			if tt.wantCode == http.StatusOK && rec.Body.String() != tt.wantBody {
				t.Errorf("GET %s served %q, want %q", tt.path, rec.Body.String(), tt.wantBody)
			}
		})
	}
}
