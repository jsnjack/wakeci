package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestLine(t *testing.T) {
	buf := &syncBuffer{}
	oldLog := requestLog
	requestLog = slog.New(slog.NewJSONHandler(buf, nil))
	t.Cleanup(func() { requestLog = oldLog })
	handler := LogMi(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeBody(L, w, []byte("12345"))
	}))

	tests := []struct {
		name      string
		target    string
		userAgent string
		want      map[string]any
		notWant   []string
	}{
		{
			name:      "feed search",
			target:    "/api/feed?q=%2Bname%3Atest_init&offset=15",
			userAgent: "curl/8.9.1",
			want:      map[string]any{"level": "INFO", "query": "offset=15&q=%2Bname%3Atest_init", "bytes": float64(5), "ua": "curl/8.9.1", "status": float64(200)},
		},
		{
			name:    "run parameters",
			target:  "/api/job/deploy/run?TOKEN=s3cr3t&q=x",
			want:    map[string]any{"query": "q=x"},
			notWant: []string{"s3cr3t", "TOKEN"},
		},
		{
			name:    "no query",
			target:  "/api/jobs/",
			notWant: []string{`"query"`, `"ua"`},
		},
		{
			name:      "long values",
			target:    "/api/feed?q=" + strings.Repeat("a", 400),
			userAgent: strings.Repeat("b", 400),
			want:      map[string]any{"query": "q=" + strings.Repeat("a", 254) + "...", "ua": strings.Repeat("b", 256) + "..."},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.buf.Reset()
			req := httptest.NewRequest("GET", tt.target, nil)
			req.Header.Del("User-Agent")
			if tt.userAgent != "" {
				req.Header.Set("User-Agent", tt.userAgent)
			}
			handler.ServeHTTP(httptest.NewRecorder(), req)
			line := strings.TrimSpace(buf.String())
			var record map[string]any
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				t.Fatalf("request line isn't JSON: %q", line)
			}
			for key, want := range tt.want {
				if record[key] != want {
					t.Errorf("%s = %v, want %v", key, record[key], want)
				}
			}
			for _, unwanted := range tt.notWant {
				if strings.Contains(line, unwanted) {
					t.Errorf("request line %s contains %s", line, unwanted)
				}
			}
		})
	}
}
