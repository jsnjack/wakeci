package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// bearer adds an API key to every request
type bearer struct {
	token string
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

// mcpSession connects an MCP client with token to the key router
func mcpSession(t *testing.T, router http.Handler, token string) *mcp.ClientSession {
	t.Helper()
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   server.URL + "/mcp",
		HTTPClient: &http.Client{Transport: bearer{token: token}},
	}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Logf("close session: %v", err)
		}
	})
	return session
}

// callTool returns the tool's text, or its error text with isError
func callTool(t *testing.T, session *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	var text strings.Builder
	for _, content := range result.Content {
		if c, ok := content.(*mcp.TextContent); ok {
			text.WriteString(c.Text)
		}
	}
	return text.String(), result.IsError
}

func TestMCPTools(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	addFailedTestBuild(t)
	token := createTestKey(t, url.Values{"name": {"agent"}, "scope": {"read"}, "job": {"deploy"}})
	session := mcpSession(t, router, token)

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	want := []string{"abort_build", "build_summary", "find_builds", "list_jobs", "read_artifact", "read_log", "run_job", "whoami"}
	if !slices.Equal(names, want) {
		t.Errorf("tools = %q, want %q", names, want)
	}

	tests := []struct {
		tool        string
		args        map[string]any
		wantError   bool
		wantContain string
	}{
		{"whoami", nil, false, `"auth":"apikey"`},
		{"list_jobs", nil, false, `"name":"deploy"`},
		{"list_jobs", map[string]any{"search": "night"}, false, `"jobs":[]`},
		{"find_builds", map[string]any{"job": "deploy"}, false, `"id":7`},
		{"find_builds", map[string]any{"status": "done"}, true, "invalid status"},
		{"build_summary", map[string]any{"id": 7, "tail": 1}, false, "FAILED test_login"},
		{"build_summary", map[string]any{"id": 2}, true, "403 Forbidden: This API key doesn't cover that job"},
		{"build_summary", map[string]any{"id": 99}, true, "build 99 not found on this server"},
		{"read_log", map[string]any{"build": 7, "task": 1, "grep": "failed"}, false, "300: FAILED test_login"},
		{"read_log", map[string]any{"build": 7, "task": 0}, false, "line 3"},
		{"read_log", map[string]any{"build": 7, "task": 2}, true, "wrote no log"},
		{"run_job", map[string]any{"name": "deploy"}, true, "doesn't have the run scope"},
		{"abort_build", map[string]any{"id": 7}, true, "doesn't have the run scope"},
	}
	for _, tt := range tests {
		args, _ := json.Marshal(tt.args)
		t.Run(tt.tool+" "+string(args), func(t *testing.T) {
			text, isError := callTool(t, session, tt.tool, tt.args)
			if isError != tt.wantError {
				t.Errorf("isError = %t, want %t; text %q", isError, tt.wantError, text)
			}
			if !strings.Contains(text, tt.wantContain) {
				t.Errorf("text = %q, want it to contain %q", text, tt.wantContain)
			}
		})
	}
}

func TestMCPRunJobAndArtifacts(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	addFailedTestBuild(t)
	token := createTestKey(t, url.Values{"name": {"agent"}, "scope": {"read", "run"}, "job": {"deploy"}})
	session := mcpSession(t, router, token)

	// A disabled job stops RunJob before it queues a build, with an error
	// that only the run handler gives
	if rec := adminRequest("POST", "/api/job/deploy/set_active", url.Values{"active": {"false"}}); rec.Code != http.StatusOK {
		t.Fatalf("disable job: %d %q", rec.Code, rec.Body.String())
	}
	if text, isError := callTool(t, session, "run_job", map[string]any{"name": "deploy", "params": map[string]any{"TARGET": "{{ secrets.TOKEN }}"}}); !isError || !strings.Contains(text, "references a secret") {
		t.Errorf("run with a secret: %q, error %t", text, isError)
	}
	if text, isError := callTool(t, session, "run_job", map[string]any{"name": "deploy"}); !isError || !strings.Contains(text, "not enabled") {
		t.Errorf("run a disabled job: %q, error %t; want the run handler's answer", text, isError)
	}

	writeArtifact(t, 7, "report.json", "{\"failed\": 1}")
	writeArtifact(t, 7, "screenshot.png", "\x89PNG\x00\x01")
	if text, isError := callTool(t, session, "read_artifact", map[string]any{"build": 7, "path": "report.json"}); isError || text != `{"failed": 1}` {
		t.Errorf("text artifact: %q, error %t", text, isError)
	}
	if text, isError := callTool(t, session, "read_artifact", map[string]any{"build": 7, "path": "screenshot.png"}); !isError || !strings.Contains(text, "isn't text") {
		t.Errorf("binary artifact: %q, error %t", text, isError)
	}
}

func TestMCPNeedsAnAPIKey(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	_, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   server.URL + "/mcp",
		HTTPClient: &http.Client{Transport: bearer{token: "wk_000000000000_wrong"}},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "Unauthorized") {
		t.Errorf("connect with a wrong key: %v, want 401", err)
	}
}

func writeArtifact(t *testing.T, build int, name, content string) {
	t.Helper()
	dir := filepath.Join(Config.WorkDir, "wakespace", strconv.Itoa(build), "artifacts")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("create artifacts: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
}

// A change made through MCP is audited like the same request to the API,
// and its request line names the MCP client
func TestMCPCallsAreAuditedAndLogged(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	token := createTestKey(t, url.Values{"name": {"agent"}, "scope": {"run"}, "job": {"deploy"}})
	keyID := strings.Split(token, "_")[1]
	session := mcpSession(t, router, token)
	buf := &syncBuffer{}
	oldLog := requestLog
	requestLog = slog.New(slog.NewJSONHandler(buf, nil))
	t.Cleanup(func() { requestLog = oldLog })
	if rec := adminRequest("POST", "/api/job/deploy/set_active", url.Values{"active": {"false"}}); rec.Code != http.StatusOK {
		t.Fatalf("disable job: %d", rec.Code)
	}

	events, _ := captureAudit(t)
	callTool(t, session, "run_job", map[string]any{"name": "deploy"})
	var run auditEvent
	for _, event := range events() {
		if event["event"] == "build.run" {
			run = event
		}
	}
	checkAuditEvent(t, run, map[string]string{"event": "build.run", "outcome": "failed", "status": "400", "auth": "apikey", "apiKey": keyID, "job": "deploy"})
	if !strings.Contains(buf.String(), `"path":"/api/job/deploy/run"`) || !strings.Contains(buf.String(), `"ua":"wakeci-mcp`) {
		t.Errorf("request lines = %s, want the run with the MCP client", buf.String())
	}
}
