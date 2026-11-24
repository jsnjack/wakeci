package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	bolt "go.etcd.io/bbolt"
)

// freshRequestStats gives the test counts of its own
func freshRequestStats(t *testing.T) {
	t.Helper()
	old := requestStats
	requestStats = &requestCounter{pending: map[string]map[requestRow]*requestCounts{}}
	t.Cleanup(func() { requestStats = old })
}

// countedToday returns today's counts that aren't saved yet
func countedToday() map[requestRow]*requestCounts {
	today := usageDay(time.Now())
	return requestStats.copyDays(today, today)[today]
}

// statsRouter routes like wakeci: requests with an API key to the key router,
// the others to the session routes and the web UI
func statsRouter(t *testing.T) http.Handler {
	t.Helper()
	keyRouter := newAPIKeyTestEnv(t)
	useQueue(t, &Queue{})
	router := chi.NewRouter()
	router.Use(LogMi)
	router.Route("/api", apiRoutes)
	router.Method("GET", spaRoute, http.NotFoundHandler())
	return routeAPIKeys(keyRouter, router)
}

func keyIDOf(token string) string {
	return strings.Split(token, "_")[1]
}

func TestRequestsAreCountedByHowTheyCameIn(t *testing.T) {
	router := statsRouter(t)
	freshRequestStats(t)
	admin := loginAs(t, adminUser)
	ann := loginAs(t, "ann@example.com")
	token := createTestKey(t, url.Values{"name": {"ci"}, "scope": {"read"}, "job": {"deploy"}})
	key := buildTrigger{Kind: TriggerAPIKey, APIKey: keyIDOf(token)}
	password := buildTrigger{Kind: TriggerPassword}

	tests := []struct {
		name    string
		target  string
		prepare func(*http.Request)
		want    *requestRow
	}{
		{"web UI", "/api/feed", func(r *http.Request) { r.AddCookie(admin) },
			&requestRow{Via: ViaWeb, Route: "GET /api/feed", Outcome: OutcomeSuccess, Who: password}},
		{"single sign-on", "/api/build/1", func(r *http.Request) { r.AddCookie(ann) },
			&requestRow{Via: ViaWeb, Route: "GET /api/build/{id}", Outcome: OutcomeSuccess, Who: buildTrigger{Kind: TriggerSSO, User: "ann@example.com"}}},
		{"no credentials", "/api/feed", nil,
			&requestRow{Via: ViaNone, Route: "GET /api/feed", Outcome: OutcomeRefused}},
		{"password", "/api/build/1", func(r *http.Request) { r.SetBasicAuth("", "admin") },
			&requestRow{Via: ViaBasic, Route: "GET /api/build/{id}", Outcome: OutcomeSuccess, Who: password}},
		{"wrong password", "/api/build/1", func(r *http.Request) { r.SetBasicAuth("", "nope") },
			&requestRow{Via: ViaBasic, Route: "GET /api/build/{id}", Outcome: OutcomeRefused}},
		{"no such route", "/api/nosuch", func(r *http.Request) { r.AddCookie(admin) },
			&requestRow{Via: ViaWeb, Outcome: OutcomeClientError, Who: password}},
		{"api key", "/api/build/1", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) },
			&requestRow{Via: ViaKey, Route: "GET /api/build/{id}", Outcome: OutcomeSuccess, Who: key}},
		{"another job's build", "/api/build/2", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) },
			&requestRow{Via: ViaKey, Route: "GET /api/build/{id}", Outcome: OutcomeRefused, Who: key}},
		{"route keys can't use", "/api/settings", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) },
			&requestRow{Via: ViaKey, Outcome: OutcomeRefused, Who: key}},
		{"wrong key", "/api/build/1", func(r *http.Request) { r.Header.Set("Authorization", "Bearer wk_nosuch_secret") },
			&requestRow{Via: ViaKey, Route: "GET /api/build/{id}", Outcome: OutcomeRefused}},
		{"web UI's files", "/favicon.ico", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestStats.takeAll()
			req := httptest.NewRequest("GET", tt.target, nil)
			if tt.prepare != nil {
				tt.prepare(req)
			}
			router.ServeHTTP(httptest.NewRecorder(), req)
			counted := countedToday()
			if tt.want == nil {
				if len(counted) != 0 {
					t.Fatalf("counted %v, want nothing", counted)
				}
				return
			}
			if len(counted) != 1 || counted[*tt.want] == nil || counted[*tt.want].N != 1 {
				for row, counts := range counted {
					t.Logf("counted %+v: %d", row, counts.N)
				}
				t.Fatalf("want one request of %+v", *tt.want)
			}
			if counted[*tt.want].Timed != 1 {
				t.Errorf("the request's time doesn't count")
			}
		})
	}
}

func TestWaitsAndWebSocketsDontCountTheirTime(t *testing.T) {
	freshRequestStats(t)
	for _, target := range []string{"/api/build/1?wait=30", "/ws", "/api/build/1?wait=0"} {
		route := strings.Split(target, "?")[0]
		if strings.HasPrefix(route, "/api/build/") {
			route = "/api/build/{id}"
		}
		countRequest(httptest.NewRequest("GET", target, nil), route, ViaWeb, 200, time.Now(), time.Minute)
	}
	counted := countedToday()
	timed := map[string]int64{}
	for row, counts := range counted {
		timed[row.Route] += counts.Timed
	}
	if timed["GET /api/build/{id}"] != 1 || timed["GET /ws"] != 0 {
		t.Errorf("timed requests %v, want only the build read without wait", timed)
	}
}

func TestMCPToolCallsAreCounted(t *testing.T) {
	router := statsRouter(t)
	freshRequestStats(t)
	buf := &syncBuffer{}
	oldLog := requestLog
	requestLog = slog.New(slog.NewJSONHandler(buf, nil))
	t.Cleanup(func() { requestLog = oldLog })
	token := createTestKey(t, url.Values{"name": {"agent"}, "scope": {"read"}, "job": {"deploy"}})
	key := buildTrigger{Kind: TriggerAPIKey, APIKey: keyIDOf(token)}
	session := mcpSession(t, router, token)

	if _, failed := callTool(t, session, "whoami", nil); failed {
		t.Fatal("whoami failed")
	}
	if _, failed := callTool(t, session, "build_summary", map[string]any{"id": 2}); !failed {
		t.Fatal("build_summary of another job's build didn't fail")
	}

	counted := countedToday()
	want := map[requestRow]int64{
		{Via: ViaMCP, Tool: "whoami", Outcome: OutcomeSuccess, Who: key}:        1,
		{Via: ViaMCP, Tool: "build_summary", Outcome: OutcomeRefused, Who: key}: 1,
	}
	for row, n := range want {
		if counted[row] == nil || counted[row].N != n {
			t.Errorf("want %d of %+v", n, row)
		}
	}
	posts := int64(0)
	for row, counts := range counted {
		if row.Via != ViaMCP {
			t.Errorf("counted %+v: the API calls of a tool count as the tool call", row)
		}
		if row.Route == "POST /mcp" {
			posts += counts.N
		}
	}
	// The session's start and the two calls at least
	if posts < 3 {
		t.Errorf("counted %d requests to /mcp, want at least 3", posts)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var sawTool, sawInner bool
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("request line isn't JSON: %q", line)
		}
		if record["path"] == "/mcp" && record["tool"] == "whoami" && record["via"] == ViaMCP && record["route"] == "/mcp" {
			sawTool = true
		}
		if record["path"] == "/api/whoami" && record["tool"] == "whoami" && record["via"] == ViaMCP && record["route"] == "/api/whoami" {
			sawInner = true
		}
	}
	if !sawTool || !sawInner {
		t.Errorf("the request lines don't name the tool of the request to /mcp (%v) and its API call (%v):\n%s", sawTool, sawInner, buf.String())
	}
}

func TestToolOutcome(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{nil, OutcomeSuccess},
		{&mcpAPIError{status: 404}, OutcomeClientError},
		{&mcpAPIError{status: 403}, OutcomeRefused},
		{&mcpAPIError{status: 500}, OutcomeServerError},
		{errString("decode"), OutcomeServerError},
	}
	for _, tt := range tests {
		if got := toolOutcome(nil, tt.err); got != tt.want {
			t.Errorf("toolOutcome(%v) = %s, want %s", tt.err, got, tt.want)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestStatusOutcome(t *testing.T) {
	for status, want := range map[int]string{
		200: OutcomeSuccess, 204: OutcomeSuccess, 304: OutcomeSuccess,
		400: OutcomeClientError, 404: OutcomeClientError, 405: OutcomeClientError, 409: OutcomeClientError,
		401: OutcomeRefused, 403: OutcomeRefused, 429: OutcomeRefused,
		500: OutcomeServerError, 502: OutcomeServerError,
	} {
		if got := statusOutcome(status); got != want {
			t.Errorf("statusOutcome(%d) = %s, want %s", status, got, want)
		}
	}
}

func TestRequestTimePercentiles(t *testing.T) {
	var none requestCounts
	if none.percentile(0.5) != 0 {
		t.Errorf("median of nothing = %v, want 0", none.percentile(0.5))
	}

	var counts requestCounts
	for range 50 {
		counts.addRequest(4*time.Millisecond, true)
		counts.addRequest(40*time.Millisecond, true)
	}
	counts.addRequest(time.Hour, false)
	if counts.N != 101 || counts.Timed != 100 || counts.Max != 40*time.Millisecond {
		t.Fatalf("counts %+v, want 101 requests, 100 timed, the slowest 40ms", counts)
	}
	if p50 := counts.percentile(0.5); p50 < 3*time.Millisecond || p50 > 5*time.Millisecond {
		t.Errorf("median %v, want in the bucket of 4ms (3ms to 5ms)", p50)
	}
	if p95 := counts.percentile(0.95); p95 < 30*time.Millisecond || p95 > 40*time.Millisecond {
		t.Errorf("p95 %v, want from 30ms to the slowest, 40ms", p95)
	}

	var slow requestCounts
	slow.addRequest(90*time.Second, true)
	if p95 := slow.percentile(0.95); p95 < 60*time.Second || p95 > 90*time.Second {
		t.Errorf("p95 of one request of 90s = %v, want from 60s to 90s", p95)
	}
}

// statsRequest asks the request stats with a session of user
func statsRequest(t *testing.T, target, user string) *httptest.ResponseRecorder {
	t.Helper()
	return apiCall("GET", target, loginAs(t, user), nil, nil)
}

func TestRequestStatsAPI(t *testing.T) {
	router := statsRouter(t)
	freshRequestStats(t)
	token := createTestKey(t, url.Values{"name": {"ci"}, "scope": {"read"}, "job": {"deploy"}})
	keyID := keyIDOf(token)
	now := time.Now()
	yesterday := usageDay(now.AddDate(0, 0, -1))
	today := usageDay(now)
	key := buildTrigger{Kind: TriggerAPIKey, APIKey: keyID}
	ann := buildTrigger{Kind: TriggerSSO, User: "ann@example.com"}
	requestStats.add(yesterday, requestRow{Via: ViaKey, Route: "GET /api/build/{id}", Outcome: OutcomeSuccess, Who: key}, 10*time.Millisecond, true)
	requestStats.add(yesterday, requestRow{Via: ViaWeb, Route: "GET /api/feed", Outcome: OutcomeSuccess, Who: ann}, 5*time.Millisecond, true)
	if err := flushRequestStats(); err != nil {
		t.Fatalf("save request stats: %v", err)
	}
	// Saved and not saved yet of the same day and row add up
	requestStats.add(yesterday, requestRow{Via: ViaKey, Route: "GET /api/build/{id}", Outcome: OutcomeServerError, Who: key}, 20*time.Millisecond, true)
	requestStats.add(today, requestRow{Via: ViaKey, Route: "GET /api/build/{id}", Outcome: OutcomeSuccess, Who: key}, 30*time.Millisecond, true)
	requestStats.add(today, requestRow{Via: ViaMCP, Tool: "read_log", Outcome: OutcomeSuccess, Who: key}, 3*time.Millisecond, true)
	requestStats.add(today, requestRow{Via: ViaMCP, Route: "POST /mcp", Outcome: OutcomeSuccess, Who: key}, 4*time.Millisecond, true)
	requestStats.add(today, requestRow{Via: ViaNone, Route: "GET /api/feed", Outcome: OutcomeRefused}, time.Millisecond, true)

	rec := statsRequest(t, "/api/analytics/requests?days=7", adminUser)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var data RequestStatsData
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if data.Days != 7 || len(data.ByDay) != 7 || data.Today != today || data.Since == nil || *data.Since != yesterday {
		t.Errorf("range %d days, %d in byDay, today %s, since %v; want 7, 7, %s, %s", data.Days, len(data.ByDay), data.Today, data.Since, today, yesterday)
	}
	wantTotals := RequestTotals{Requests: 6, RequestOutcomes: RequestOutcomes{Success: 4, Refused: 1, ServerError: 1},
		ToolCalls: 1, ToolOutcomes: RequestOutcomes{Success: 1}}
	if data.Totals != wantTotals {
		t.Errorf("totals %+v, want %+v", data.Totals, wantTotals)
	}
	if last := data.ByDay[6]; last.Day != today || last.Requests != 3 || last.ToolCalls != 1 || last.Refused != 1 {
		t.Errorf("today %+v, want 3 requests (1 refused) and 1 tool call", last)
	}
	if first := data.Routes[0]; first.Via != ViaKey || first.Route != "GET /api/build/{id}" || first.Requests != 3 || first.ServerError != 1 ||
		first.Time.Count != 3 || first.Time.Max != 30*time.Millisecond {
		t.Errorf("busiest route %+v, want GET /api/build/{id} with a key, 3 requests, 1 server error, slowest 30ms", first)
	}
	if len(data.Tools) != 1 || data.Tools[0].Tool != "read_log" || data.Tools[0].Calls != 1 {
		t.Errorf("tools %+v, want one call of read_log", data.Tools)
	}
	if len(data.People) != 2 || data.People[0].ID != "apikey:"+keyID || data.People[0].KeyName != "ci" ||
		data.People[0].Requests != 3+1 || data.People[0].ToolCalls != 1 || data.People[0].ByVia[ViaMCP] != 1 {
		t.Errorf("people %+v, want the key ci (4 requests, 1 tool call) first, then ann", data.People)
	}

	// A filter keeps the choices of the other: every way in for a person,
	// everyone for a way in
	rec = statsRequest(t, "/api/analytics/requests?days=7&via=web", adminUser)
	data = RequestStatsData{}
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if data.Totals.Requests != 1 || len(data.Interfaces) != 4 || len(data.People) != 1 || data.People[0].User != "ann@example.com" || data.Totals.ToolCalls != 0 {
		t.Errorf("via=web: totals %+v, interfaces %+v, people %+v; want ann's 1 request, all 4 ways in", data.Totals, data.Interfaces, data.People)
	}
	rec = statsRequest(t, "/api/analytics/requests?days=7&who=sso:ann@example.com", adminUser)
	data = RequestStatsData{}
	if err := json.Unmarshal(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if data.Totals.Requests != 1 || len(data.People) != 2 || len(data.Interfaces) != 1 {
		t.Errorf("who=ann: totals %+v, people %d, interfaces %+v; want 1 request, both people, ann's one way in", data.Totals, len(data.People), data.Interfaces)
	}

	for _, tt := range []struct {
		target string
		status int
	}{
		{"/api/analytics/requests?days=0", http.StatusBadRequest},
		{"/api/analytics/requests?via=robot", http.StatusBadRequest},
	} {
		if rec := statsRequest(t, tt.target, adminUser); rec.Code != tt.status {
			t.Errorf("%s: status %d, want %d", tt.target, rec.Code, tt.status)
		}
	}
	if rec := statsRequest(t, "/api/analytics/requests", "ann@example.com"); rec.Code != http.StatusForbidden {
		t.Errorf("a single sign-on user got %d, want 403", rec.Code)
	}
	all := createTestKey(t, url.Values{"name": {"all"}, "scope": {"read", "run", "write"}, "allJobs": {"true"}})
	if rec := keyRequest(router, "GET", "/api/analytics/requests", all, nil); rec.Code != http.StatusForbidden {
		t.Errorf("an API key got %d, want 403", rec.Code)
	}
}

func TestPruneRequestStats(t *testing.T) {
	newTestBuildEnv(t)
	now := time.Now()
	old := usageDay(now.Add(-requestStatsRetention - 48*time.Hour))
	kept := usageDay(now.Add(-requestStatsRetention + 48*time.Hour))
	err := DB.Update(func(tx *bolt.Tx) error {
		for _, day := range []string{old, kept} {
			if err := tx.Bucket(RequestStatsBucket).Put([]byte(day), []byte(`{"rows":[]}`)); err != nil {
				return err
			}
		}
		return pruneRequestStats(tx, now, L)
	})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	err = DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(RequestStatsBucket)
		if b.Get([]byte(old)) != nil || b.Get([]byte(kept)) == nil {
			t.Errorf("after pruning: %s kept %v, %s kept %v; want only %s", old, b.Get([]byte(old)) != nil, kept, b.Get([]byte(kept)) != nil, kept)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFlushRequestStatsKeepsCountsItCantSave(t *testing.T) {
	newTestBuildEnv(t)
	freshRequestStats(t)
	row := requestRow{Via: ViaWeb, Route: "GET /api/feed", Outcome: OutcomeSuccess}
	requestStats.add(usageDay(time.Now()), row, time.Millisecond, true)
	if err := DB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := flushRequestStats(); err == nil {
		t.Fatal("saving to a closed database didn't fail")
	}
	if counted := countedToday(); counted[row] == nil || counted[row].N != 1 {
		t.Errorf("after a failed save the counts are %v, want the request back", counted)
	}
}
