package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestPreviewTokens(t *testing.T) {
	newAuthTestEnv(t)
	key, _ := mustCreateAPIKey(t, passwordPrincipal(viaSession), []Scope{ScopeRead}, 0, time.Now())
	now := time.Now()
	cookie, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodGet, "/", nil), passwordPrincipal(viaSession))
	if err != nil {
		t.Fatal(err)
	}
	request := withPrincipal(httptest.NewRequest(http.MethodGet, "/", nil), passwordPrincipal(viaSession))
	request.AddCookie(cookie)
	session, err := newPreviewToken(request, 7, "/7/artifacts/index.html", now)
	if err != nil {
		t.Fatal(err)
	}
	keyToken, err := newPreviewToken(withPrincipal(httptest.NewRequest(http.MethodGet, "/", nil), &Principal{ID: key.OwnerID, Provider: key.OwnerProvider, KeyID: key.ID, Via: viaAPIKey}), 7, "/7/artifacts/index.html", now)
	if err != nil {
		t.Fatal(err)
	}
	payload, signature, _ := strings.Cut(session, ".")

	tests := []struct {
		name    string
		token   string
		at      time.Time
		setup   func()
		wantErr bool
	}{
		{name: "valid", token: session, at: now},
		{name: "valid for a key", token: keyToken, at: now},
		{name: "expired", token: session, at: now.Add(previewTTL), wantErr: true},
		{name: "tampered payload", token: payload + "x." + signature, at: now, wantErr: true},
		{name: "no signature", token: payload, at: now, wantErr: true},
		{name: "key revoked", token: keyToken, at: now, setup: func() {
			if err := RevokeAPIKey(key.OwnerID, key.ID); err != nil {
				t.Fatal(err)
			}
		}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.setup()
			}
			claims, err := checkPreviewToken(tt.token, tt.at)
			if errors.Is(err, errInvalidPreview) != tt.wantErr || (err != nil && !tt.wantErr) {
				t.Fatalf("checkPreviewToken: %v, want errInvalidPreview: %t", err, tt.wantErr)
			}
			if err == nil && claims.Build != 7 {
				t.Errorf("build = %d, want 7", claims.Build)
			}
		})
	}
}

// TestArtifactsAreSandboxed: build files are task output, so they're only
// served sandboxed, and HTML goes through a preview URL from which its own
// files load without the session cookie
func TestArtifactsAreSandboxed(t *testing.T) {
	newJobAccessTestEnv(t)
	report := Config.WorkDir + "wakespace/1/artifacts/report/"
	if err := os.MkdirAll(report, 0755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"index.html": "<p>report</p>", "style.css": "p {}"} {
		if err := os.WriteFile(report+name, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cookie, err := GlobalSessionStorage.New(httptest.NewRequest(http.MethodGet, "/", nil), passwordPrincipal(viaSession))
	if err != nil {
		t.Fatal(err)
	}
	get := func(target string, withSession bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if withSession {
			req.AddCookie(cookie)
		}
		rec := httptest.NewRecorder()
		newRouter().ServeHTTP(rec, req)
		return rec
	}
	sandboxed := func(t *testing.T, rec *httptest.ResponseRecorder) {
		t.Helper()
		csp := rec.Header().Get("content-security-policy")
		if !strings.HasPrefix(csp, "sandbox ") || strings.Contains(csp, "allow-same-origin") {
			t.Errorf("CSP = %q, want a sandbox without allow-same-origin", csp)
		}
	}

	log := get("/storage/build/1/task_0.log", true)
	if log.Code != http.StatusOK || log.Body.String() != "test_unit log" {
		t.Errorf("log: status = %d body = %q", log.Code, log.Body.String())
	}
	sandboxed(t, log)

	for _, page := range []string{"/storage/build/1/artifacts/report/index.html", "/storage/build/1/artifacts/report/"} {
		redirect := get(page, true)
		sandboxed(t, redirect)
		location := redirect.Header().Get("Location")
		if redirect.Code != http.StatusFound || !strings.HasPrefix(location, "/preview/") {
			t.Fatalf("%s: status = %d location = %q, want a redirect to a preview", page, redirect.Code, location)
		}
		// The sandboxed page has no cookie to send
		dir := location[:strings.LastIndex(location, "/")+1]
		for _, file := range []string{dir, dir + "style.css"} {
			rec := get(file, false)
			if rec.Code != http.StatusOK {
				t.Errorf("%s: status = %d (%s), want 200", file, rec.Code, rec.Body.String())
			}
			sandboxed(t, rec)
		}
		token := strings.Split(location, "/")[2]
		for _, other := range []string{"/preview/" + token + "/build/2/task_0.log", "/preview/" + token + "/build/1/../2/task_0.log", "/preview/" + token + "/build/1%2F..%2F2/task_0.log"} {
			if rec := get(other, false); rec.Code != http.StatusNotFound {
				t.Errorf("%s: status = %d, want 404 outside the token's build", other, rec.Code)
			}
		}
	}

	if rec := get("/preview/forged.token/build/1/artifacts/report/index.html", false); rec.Code != http.StatusForbidden {
		t.Errorf("forged token: status = %d, want 403", rec.Code)
	}
	if rec := get("/storage/build/1/artifacts/report/index.html", false); rec.Code != http.StatusForbidden {
		t.Errorf("no session: status = %d, want 403", rec.Code)
	}
}

func TestPreviewConfinesSymlinksToSignedDirectory(t *testing.T) {
	newJobAccessTestEnv(t)
	build := Config.WorkDir + "wakespace/1/"
	for _, dir := range []string{"artifacts/assets", "private"} {
		if err := os.MkdirAll(build+dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for file, data := range map[string]string{"artifacts/assets/style.css": "report style", "private/hidden.txt": "PRIVATE-CREDENTIAL"} {
		if err := os.WriteFile(build+file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range map[string]string{"artifacts/link": "../private", "artifacts/local": "assets", "artifacts/hidden.txt": "../private/hidden.txt"} {
		if err := os.Symlink(target, build+link); err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	cookie, err := GlobalSessionStorage.New(request, passwordPrincipal(viaSession))
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(cookie)
	request = withPrincipal(request, passwordPrincipal(viaSession))
	token, err := newPreviewToken(request, 1, "/1/artifacts/index.html", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path   string
		status int
		body   string
	}{
		{"assets/style.css", http.StatusOK, "report style"},
		{"local/style.css", http.StatusOK, "report style"},
		{"link/hidden.txt", http.StatusNotFound, ""},
		{"hidden.txt", http.StatusNotFound, ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/preview/"+token+"/build/1/artifacts/"+tc.path, nil))
			if rec.Code != tc.status || (tc.body != "" && rec.Body.String() != tc.body) || strings.Contains(rec.Body.String(), "PRIVATE-CREDENTIAL") {
				t.Fatalf("status %d body %q; want %d %q", rec.Code, rec.Body.String(), tc.status, tc.body)
			}
		})
	}
}

func TestPreviewCORSWithConfiguredHostname(t *testing.T) {
	newJobAccessTestEnv(t)
	Config.Hostname = "wakeci.example"
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	cookie, err := GlobalSessionStorage.New(request, passwordPrincipal(viaSession))
	if err != nil {
		t.Fatal(err)
	}
	request.AddCookie(cookie)
	request = withPrincipal(request, passwordPrincipal(viaSession))
	token, err := newPreviewToken(request, 1, "/1/task_0.log", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, path, origin string
		authenticated      bool
	}{
		{"preview", "/preview/" + token + "/build/1/task_0.log", "*", false},
		{"storage", "/storage/build/1/task_0.log", "https://wakeci.example", true},
		{"application", "/auth/methods", "https://wakeci.example", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Origin", "null")
			if tc.authenticated {
				req.AddCookie(cookie)
			}
			rec := httptest.NewRecorder()
			newRouter().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.origin {
				t.Errorf("CORS origin %q, want %q", got, tc.origin)
			}
			if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
				t.Error("unexpected credentialed CORS")
			}
		})
	}
}

func TestPreviewRedirectPreservesQuery(t *testing.T) {
	newJobAccessTestEnv(t)
	dir := Config.WorkDir + "wakespace/1/artifacts/"
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	name := "report #?%.html"
	if err := os.WriteFile(dir+name, []byte("report"), 0600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	cookie, err := GlobalSessionStorage.New(request, passwordPrincipal(viaSession))
	if err != nil {
		t.Fatal(err)
	}
	rawQuery := "filter=failed&filter=queued&name=a%2Bb&space=x+y"
	request = httptest.NewRequest(http.MethodGet, "/storage/build/1/artifacts/"+url.PathEscape(name)+"?"+rawQuery, nil)
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	newRouter().ServeHTTP(response, request)
	if response.Code != http.StatusFound {
		t.Fatalf("status %d", response.Code)
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if location.RawQuery != rawQuery {
		t.Errorf("query = %q, want %q", location.RawQuery, rawQuery)
	}
	if !strings.HasSuffix(location.Path, "/artifacts/"+name) || location.Fragment != "" {
		t.Fatalf("incorrect redirect path %q or fragment %q", location.Path, location.Fragment)
	}
	result := httptest.NewRecorder()
	newRouter().ServeHTTP(result, httptest.NewRequest(http.MethodGet, location.String(), nil))
	if result.Code != http.StatusOK || result.Body.String() != "report" {
		t.Fatalf("preview: %d %q", result.Code, result.Body.String())
	}
}
