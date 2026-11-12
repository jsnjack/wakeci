package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPercentiles(t *testing.T) {
	s := time.Second
	if got := percentiles([]time.Duration{4 * s, 1 * s, 3 * s, 2 * s, 5 * s}); got != (Percentiles{Count: 5, P50: 3 * s, P95: 4800 * time.Millisecond}) {
		t.Errorf("five values: %+v", got)
	}
	if got := percentiles([]time.Duration{7 * s}); got != (Percentiles{Count: 1, P50: 7 * s, P95: 7 * s}) {
		t.Errorf("one value: %+v", got)
	}
	if got := percentiles(nil); got != (Percentiles{}) {
		t.Errorf("no values: %+v", got)
	}
}

func TestFailingStreak(t *testing.T) {
	builds := []*buildStats{
		{ID: 1, Status: StatusFinished},
		{ID: 2, Status: StatusFailed, FailedTask: "Test"},
		// A person stopped it; the streak goes on
		{ID: 3, Status: StatusAborted},
		{ID: 4, Status: StatusTimedOut, FailedTask: "Deploy"},
	}
	got := failingStreak(builds)
	if got == nil || got.Builds != 2 || got.First.ID != 2 || got.Last.ID != 4 || got.Last.FailedTask != "Deploy" {
		t.Errorf("streak = %+v, want builds 2 to 4", got)
	}
	if got := failingStreak(append(builds, &buildStats{ID: 5, Status: StatusFinished})); got != nil {
		t.Errorf("streak after a finished build = %+v, want none", got)
	}
	if got := failingStreak([]*buildStats{{ID: 1, Status: StatusAborted}}); got != nil {
		t.Errorf("only aborted = %+v, want none", got)
	}
}

// analyticsTestRecords are builds around a week that ends on Thursday
// 2026-11-12 in a zone two hours ahead of UTC
func analyticsTestRecords(loc *time.Location) []*buildStats {
	at := func(day, hour, minute int) time.Time { return time.Date(2026, 11, day, hour, minute, 0, 0, loc) }
	at10 := func(month time.Month, day int) time.Time { return time.Date(2026, month, day, 12, 0, 0, 0, loc) }
	return []*buildStats{
		{ID: 1, Job: "a", Status: StatusFinished, QueuedAt: at(12, 9, 0), StartedAt: at(12, 9, 1), Duration: 10 * time.Minute, Ended: true,
			Trigger: buildTrigger{Kind: TriggerAPIKey, User: "sam@example.com", APIKey: "k1"}},
		{ID: 2, Job: "a", Status: StatusFailed, QueuedAt: at(12, 10, 0), StartedAt: at(12, 10, 0).Add(30 * time.Second), Duration: 2 * time.Minute, Ended: true,
			FailedTask: "Test", Trigger: buildTrigger{Kind: TriggerSSO, User: "sam@example.com"}},
		{ID: 3, Job: "a", Status: StatusAborted, QueuedAt: at(12, 11, 0), Ended: true, Trigger: buildTrigger{Kind: TriggerPassword}},
		{ID: 4, Job: "b", Status: StatusTimedOut, StartedAt: at(11, 23, 30), Duration: time.Hour, Ended: true, Trigger: buildTrigger{Kind: TriggerInterval}},
		// 22:30 UTC the day before, which is the range's first day here
		{ID: 5, Job: "b", Status: StatusFinished, StartedAt: time.Date(2026, 11, 5, 22, 30, 0, 0, time.UTC), Duration: 20 * time.Minute, Ended: true},
		// In the first day's rolling window and the days before the range
		{ID: 6, Job: "a", Status: StatusFinished, StartedAt: at10(11, 1), Duration: time.Minute, Ended: true},
		{ID: 7, Job: "a", Status: StatusFailed, StartedAt: at10(10, 31), Duration: time.Minute, Ended: true},
		// Not over yet
		{ID: 8, Job: "b", Status: StatusRunning, QueuedAt: at(12, 12, 0)},
		// Finished, but its finally tasks still run: no duration yet
		{ID: 9, Job: "a", Status: StatusFinished, StartedAt: at(12, 12, 0)},
		// Before everything the range needs
		{ID: 10, Job: "c", Status: StatusFinished, StartedAt: at10(10, 1), Duration: time.Minute, Ended: true},
	}
}

func TestComputeAnalytics(t *testing.T) {
	loc := time.FixedZone("UTC+2", 2*3600)
	now := time.Date(2026, 11, 12, 15, 0, 0, 0, loc)
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, loc)
	data := computeAnalytics(analyticsTestRecords(loc), analyticsQuery{days: 7}, now, since, true)

	if data.From != "2026-11-06" || data.Today != "2026-11-12" || len(data.ByDay) != 7 {
		t.Fatalf("range %s to %s, %d days", data.From, data.Today, len(data.ByDay))
	}
	totals := data.Totals
	if totals.Builds != 6 || totals.Finished != 3 || totals.Failed != 1 || totals.TimedOut != 1 || totals.Aborted != 1 {
		t.Errorf("totals = %+v, want 6 builds: 3 finished and one each failed, timed out, aborted", totals.StatusCounts)
	}
	if totals.SuccessRate == nil || *totals.SuccessRate != 0.6 {
		t.Errorf("success rate = %v, want 3 of 5 (aborted left out)", totals.SuccessRate)
	}
	if totals.Duration != (Percentiles{Count: 2, P50: 15 * time.Minute, P95: 19*time.Minute + 30*time.Second}) {
		t.Errorf("duration = %+v, want the two finished builds that ended", totals.Duration)
	}
	if totals.Wait != (Percentiles{Count: 2, P50: 45 * time.Second, P95: 58500 * time.Millisecond}) {
		t.Errorf("wait = %+v, want those with a queue time", totals.Wait)
	}
	if data.PreviousSuccessRate == nil || *data.PreviousSuccessRate != 0.5 {
		t.Errorf("previous success rate = %v, want 1 of 2", data.PreviousSuccessRate)
	}

	first, last := data.ByDay[0], data.ByDay[6]
	if first.Day != "2026-11-06" || first.Finished != 1 || first.Failed != 0 {
		t.Errorf("first day = %+v, want the build at 00:30 local", first)
	}
	// Its window has builds 7, 6 and 5
	if first.Rate == nil || *first.Rate != 2.0/3 {
		t.Errorf("first day's rolling rate = %v, want 2 of 3", first.Rate)
	}
	if last.Finished != 2 || last.Failed != 1 || last.Aborted != 1 || data.ByDay[5].TimedOut != 1 {
		t.Errorf("last days = %+v %+v", data.ByDay[5], last)
	}

	// Monday first: Wednesday is 2, Thursday 3, Friday 4
	if data.Starts[3][9] != 1 || data.Starts[3][10] != 1 || data.Starts[4][0] != 1 || data.Starts[2][23] != 1 {
		t.Errorf("starts = %v", data.Starts)
	}

	wantTriggers := []TriggerCount{{TriggerUnknown, 2}, {TriggerInterval, 1}, {TriggerPassword, 1}, {TriggerSSO, 1}, {TriggerAPIKey, 1}}
	if !equalJSON(data.Triggers, wantTriggers) {
		t.Errorf("triggers = %+v, want %+v", data.Triggers, wantTriggers)
	}
	wantPeople := []PersonCount{{Kind: TriggerSSO, User: "sam@example.com", Builds: 1}, {Kind: TriggerAPIKey, User: "sam@example.com", APIKey: "k1", Builds: 1}}
	if !data.ShowsPeople || !equalJSON(data.People, wantPeople) {
		t.Errorf("people = %+v, want %+v", data.People, wantPeople)
	}

	if len(data.Jobs) != 2 || data.Jobs[0].Name != "b" || data.Jobs[1].Name != "a" {
		t.Fatalf("jobs = %+v, want b (50%%) before a (67%%)", data.Jobs)
	}
	a, b := data.Jobs[1], data.Jobs[0]
	if a.Builds != 4 || a.BuildTime != 12*time.Minute || a.Failing != nil || len(a.Recent) != 4 || a.Recent[0].ID != 1 {
		t.Errorf("job a = %+v", a)
	}
	if b.Failing == nil || b.Failing.Builds != 1 || b.Failing.Last.ID != 4 {
		t.Errorf("job b failing = %+v, want build 4", b.Failing)
	}

	// Without the admin, no one is named
	hidden := computeAnalytics(analyticsTestRecords(loc), analyticsQuery{days: 7}, now, since, false)
	if hidden.ShowsPeople || len(hidden.People) != 0 || len(hidden.Triggers) != 5 {
		t.Errorf("without the admin: people %+v, triggers %+v", hidden.People, hidden.Triggers)
	}
	// Stats that start within the days before the range can't be compared
	late := computeAnalytics(analyticsTestRecords(loc), analyticsQuery{days: 7}, now, time.Date(2026, 11, 3, 0, 0, 0, 0, loc), false)
	if late.PreviousSuccessRate != nil {
		t.Errorf("previous rate with stats since Nov 3 = %v, want null", *late.PreviousSuccessRate)
	}
}

func TestComputeAnalyticsFilters(t *testing.T) {
	loc := time.FixedZone("UTC+2", 2*3600)
	now := time.Date(2026, 11, 12, 15, 0, 0, 0, loc)
	tests := []struct {
		q    analyticsQuery
		want int
	}{
		{analyticsQuery{days: 7, job: "b"}, 2},
		{analyticsQuery{days: 7, trigger: TriggerUnknown}, 2},
		{analyticsQuery{days: 7, trigger: TriggerSSO}, 1},
		{analyticsQuery{days: 1}, 4},
		{analyticsQuery{days: 90}, 9},
	}
	for _, tt := range tests {
		if got := computeAnalytics(analyticsTestRecords(loc), tt.q, now, time.Time{}, false).Totals.Builds; got != tt.want {
			t.Errorf("%+v: %d builds, want %d", tt.q, got, tt.want)
		}
	}
}

func TestComputeAnalyticsRecentBuilds(t *testing.T) {
	now := time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC)
	var records []*buildStats
	for id := 1; id <= 25; id++ {
		records = append(records, &buildStats{ID: id, Job: "a", Status: StatusFinished, StartedAt: now.Add(time.Duration(id-30) * time.Minute)})
	}
	recent := computeAnalytics(records, analyticsQuery{days: 1}, now, time.Time{}, false).Jobs[0].Recent
	if len(recent) != 20 || recent[0].ID != 6 || recent[19].ID != 25 {
		t.Errorf("recent = %d builds from %d, want the last 20, oldest first", len(recent), recent[0].ID)
	}
}

// A day of the range is a calendar day, also across a change of the clocks
func TestAnalyticsDaysAcrossSummerTime(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Amsterdam")
	if err != nil {
		t.Skipf("no time zone data: %v", err)
	}
	// The clocks went back on 2026-10-25
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, loc)
	records := []*buildStats{
		{ID: 1, Job: "a", Status: StatusFinished, StartedAt: time.Date(2026, 10, 25, 23, 30, 0, 0, loc)},
		{ID: 2, Job: "a", Status: StatusFinished, StartedAt: time.Date(2026, 10, 26, 0, 30, 0, 0, loc)},
	}
	days := computeAnalytics(records, analyticsQuery{days: 3}, now, time.Time{}, false).ByDay
	if days[0].Day != "2026-10-25" || days[0].Finished != 1 || days[1].Finished != 1 {
		t.Errorf("days = %+v, want one build on each of Oct 25 and 26", days)
	}
}

func equalJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func TestAnalyticsQuery(t *testing.T) {
	for _, bad := range []string{"days=0", "days=366", "days=x", "trigger=robot"} {
		values, _ := url.ParseQuery(bad)
		if _, err := analyticsQueryFrom(values); err == nil {
			t.Errorf("%s: no error", bad)
		}
	}
	values, _ := url.ParseQuery("days=90&job=deploy&trigger=apikey")
	if q, err := analyticsQueryFrom(values); err != nil || q != (analyticsQuery{days: 90, job: "deploy", trigger: TriggerAPIKey}) {
		t.Errorf("query = %+v, %v", q, err)
	}
	if q, _ := analyticsQueryFrom(url.Values{}); q.days != defaultAnalyticsDays {
		t.Errorf("default days = %d", q.days)
	}
}

// The analytics count only the jobs the caller may read, and only the admin
// sees who started builds
func TestAnalyticsAPI(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	token := createTestKey(t, url.Values{"name": {"ci-bot"}, "scope": {"read"}, "job": {"deploy"}})
	keyID := strings.Split(token, "_")[1]
	now := time.Now()
	putTestStats(t,
		&buildStats{ID: 10, Job: "deploy", Status: StatusFinished, QueuedAt: now.Add(-2 * time.Hour), Ended: true,
			Trigger: buildTrigger{Kind: TriggerAPIKey, APIKey: keyID}},
		&buildStats{ID: 11, Job: "nightly", Status: StatusFailed, QueuedAt: now.Add(-time.Hour), Ended: true,
			Trigger: buildTrigger{Kind: TriggerSSO, User: "sam@example.com"}},
		// Older than the 30 days asked for, and than the slack past them
		&buildStats{ID: 9, Job: "deploy", Status: StatusFinished, QueuedAt: now.AddDate(0, 0, -40), Ended: true},
	)
	addTestGroup(t, "Deployers", Grant{Scopes: []string{ScopeRead}, Jobs: []string{"deploy"}})

	get := func(t *testing.T, rec interface {
		Result() *http.Response
	}) AnalyticsData {
		t.Helper()
		resp := rec.Result()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status %d", resp.StatusCode)
		}
		var data AnalyticsData
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return data
	}
	jobs := func(data AnalyticsData) []string {
		var names []string
		for _, job := range data.Jobs {
			names = append(names, job.Name)
		}
		return names
	}

	admin := get(t, adminRequest("GET", "/api/analytics/?days=30", nil))
	if admin.Totals.Builds != 2 || len(admin.Jobs) != 2 || !admin.ShowsPeople || admin.Since == nil {
		t.Errorf("admin: %d builds, jobs %v, people shown %t", admin.Totals.Builds, jobs(admin), admin.ShowsPeople)
	}
	var keyName string
	for _, person := range admin.People {
		if person.APIKey == keyID {
			keyName = person.KeyName
		}
	}
	if keyName != "ci-bot" {
		t.Errorf("people = %+v, want the key by its name", admin.People)
	}

	deployer := get(t, apiCall("GET", "/api/analytics/", loginInGroups(t, "dee@example.com", "Deployers"), nil, nil))
	if names := jobs(deployer); len(names) != 1 || names[0] != "deploy" || deployer.Totals.Builds != 1 || deployer.ShowsPeople || len(deployer.People) != 0 {
		t.Errorf("single sign-on user: jobs %v, %d builds, people %+v", names, deployer.Totals.Builds, deployer.People)
	}

	key := get(t, keyRequest(router, "GET", "/api/analytics/?trigger=apikey", token, nil))
	if names := jobs(key); len(names) != 1 || names[0] != "deploy" || key.ShowsPeople {
		t.Errorf("API key: jobs %v", names)
	}

	if rec := adminRequest("GET", "/api/analytics/?days=0", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("days=0: %d", rec.Code)
	}
	runOnly := createTestKey(t, url.Values{"name": {"runner"}, "scope": {"run"}, "allJobs": {"true"}})
	if rec := keyRequest(router, "GET", "/api/analytics/", runOnly, nil); rec.Code != http.StatusForbidden {
		t.Errorf("key without read: %d, want 403", rec.Code)
	}
}
