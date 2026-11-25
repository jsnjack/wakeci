package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sasha-s/go-deadlock"
	bolt "go.etcd.io/bbolt"
)

// Request stats count how people and programs use wakeci: every HTTP request
// by how it came in (via), its route, who made it and its outcome, with its
// time in buckets, and every MCP tool call the same way. Counts are per day
// in the server's time zone. They are kept in memory and saved to
// RequestStatsBucket once a minute and when wakeci stops, so counting costs
// no database write per request; a crash loses at most the last minute.
// Only write and the admin password read them (HandleRequestStats).

// requestStatsRetention is how long a day's counts are kept
const requestStatsRetention = statsRetention

// requestStatsFlushInterval is how often the counts are saved
const requestStatsFlushInterval = time.Minute

// How a request came in, by the credentials it carried, whether or not they
// were right
const (
	// ViaWeb is a session cookie: the web UI
	ViaWeb = "web"
	// ViaBasic is Basic auth with the admin password: scripts using the API
	ViaBasic = "basic"
	// ViaKey is an API key on a REST route
	ViaKey = "key"
	// ViaMCP is an API key on /mcp, and the tool calls made there
	ViaMCP = "mcp"
	// ViaBuild is a build's token: wake, run by a task
	ViaBuild = "build"
	// ViaNone is no credentials: the login page, the docs, refused requests
	ViaNone = "none"
)

// viaKinds are the ways in a query may ask for
var viaKinds = []string{ViaWeb, ViaBasic, ViaKey, ViaMCP, ViaBuild, ViaNone}

// The outcome of a request, by its status, or of a tool call
const (
	// OutcomeSuccess is a status below 400
	OutcomeSuccess = "success"
	// OutcomeRefused is 401, 403 or 429: wrong credentials, no permission or
	// too many requests
	OutcomeRefused = "refused"
	// OutcomeClientError is any other 4xx, such as 400 or 404
	OutcomeClientError = "clientError"
	// OutcomeServerError is 5xx
	OutcomeServerError = "serverError"
)

// statusOutcome names the outcome of a response status
func statusOutcome(status int) string {
	switch {
	case status < 400:
		return OutcomeSuccess
	case status == http.StatusUnauthorized, status == http.StatusForbidden, status == http.StatusTooManyRequests:
		return OutcomeRefused
	case status < 500:
		return OutcomeClientError
	}
	return OutcomeServerError
}

// requestTimeBounds are the upper bounds of the time buckets; a last bucket
// holds the requests that took longer. Each bound is at most twice the one
// before, so a percentile read from them is off by less than that.
var requestTimeBounds = []time.Duration{
	1 * time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond, 5 * time.Millisecond, 7 * time.Millisecond,
	10 * time.Millisecond, 15 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond, 50 * time.Millisecond,
	70 * time.Millisecond, 100 * time.Millisecond, 150 * time.Millisecond, 200 * time.Millisecond, 300 * time.Millisecond,
	500 * time.Millisecond, 700 * time.Millisecond, 1 * time.Second, 1500 * time.Millisecond, 2 * time.Second,
	3 * time.Second, 5 * time.Second, 7 * time.Second, 10 * time.Second, 15 * time.Second,
	20 * time.Second, 30 * time.Second, 60 * time.Second,
}

// timeBucket returns the bucket of a request that took d
func timeBucket(d time.Duration) int {
	i, _ := slices.BinarySearch(requestTimeBounds, d)
	return i
}

// requestRow is what a count is of. The zero Who is a request whose
// credentials were missing or wrong.
type requestRow struct {
	Via string `json:"via"`
	// Route is the route's method and pattern, such as GET /api/build/{id};
	// empty for a request no route matches, or a tool call
	Route string `json:"route,omitempty"`
	// Tool is the MCP tool of a tool call
	Tool    string       `json:"tool,omitempty"`
	Outcome string       `json:"outcome"`
	Who     buildTrigger `json:"who,omitzero"`
}

// requestCounts are the requests of a row
type requestCounts struct {
	N int64 `json:"n"`
	// Timed counts the requests whose time counts: all but WebSockets and
	// reads that waited for a build to end
	Timed int64         `json:"timed,omitempty"`
	Max   time.Duration `json:"max,omitempty"`
	// Buckets counts the timed requests by requestTimeBounds, without the
	// empty buckets at the end
	Buckets []int64 `json:"buckets,omitempty"`
}

// addRequest counts a request that took d; timed says whether its time counts
func (c *requestCounts) addRequest(d time.Duration, timed bool) {
	c.N++
	if !timed {
		return
	}
	c.Timed++
	c.Max = max(c.Max, d)
	i := timeBucket(d)
	if len(c.Buckets) <= i {
		c.Buckets = append(c.Buckets, make([]int64, i+1-len(c.Buckets))...)
	}
	c.Buckets[i]++
}

// merge adds other's requests
func (c *requestCounts) merge(other *requestCounts) {
	c.N += other.N
	c.Timed += other.Timed
	c.Max = max(c.Max, other.Max)
	if len(c.Buckets) < len(other.Buckets) {
		c.Buckets = append(c.Buckets, make([]int64, len(other.Buckets)-len(c.Buckets))...)
	}
	for i, n := range other.Buckets {
		c.Buckets[i] += n
	}
}

// percentile estimates the q quantile of the timed requests, interpolated
// within its bucket and never above the slowest
func (c *requestCounts) percentile(q float64) time.Duration {
	if c.Timed == 0 {
		return 0
	}
	rank := q * float64(c.Timed)
	var below int64
	for i, n := range c.Buckets {
		if n == 0 || float64(below+n) < rank {
			below += n
			continue
		}
		var lo time.Duration
		if i > 0 {
			lo = requestTimeBounds[i-1]
		}
		hi := c.Max
		if i < len(requestTimeBounds) {
			hi = min(hi, requestTimeBounds[i])
		}
		lo = min(lo, hi)
		return lo + time.Duration(float64(hi-lo)*(rank-float64(below))/float64(n))
	}
	return c.Max
}

// requestCounter holds the counts not saved yet, by day and row
type requestCounter struct {
	mu      deadlock.Mutex
	pending map[string]map[requestRow]*requestCounts
}

// requestStats counts wakeci's requests
var requestStats = &requestCounter{pending: map[string]map[requestRow]*requestCounts{}}

// add counts a request of row on day that took d
func (c *requestCounter) add(day string, row requestRow, d time.Duration, timed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := c.pending[day]
	if rows == nil {
		rows = map[requestRow]*requestCounts{}
		c.pending[day] = rows
	}
	counts := rows[row]
	if counts == nil {
		counts = &requestCounts{}
		rows[row] = counts
	}
	counts.addRequest(d, timed)
}

// takeAll removes and returns the counts not saved yet
func (c *requestCounter) takeAll() map[string]map[requestRow]*requestCounts {
	c.mu.Lock()
	defer c.mu.Unlock()
	pending := c.pending
	c.pending = map[string]map[requestRow]*requestCounts{}
	return pending
}

// putBack returns counts that couldn't be saved
func (c *requestCounter) putBack(days map[string]map[requestRow]*requestCounts) {
	c.mu.Lock()
	defer c.mu.Unlock()
	mergeRequestDays(c.pending, days)
}

// copyDays returns a copy of the counts not saved yet of the days from
// first to last
func (c *requestCounter) copyDays(first, last string) map[string]map[requestRow]*requestCounts {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := map[string]map[requestRow]*requestCounts{}
	for day, rows := range c.pending {
		if day < first || day > last {
			continue
		}
		copied[day] = map[requestRow]*requestCounts{}
		for row, counts := range rows {
			copied[day][row] = &requestCounts{N: counts.N, Timed: counts.Timed, Max: counts.Max, Buckets: slices.Clone(counts.Buckets)}
		}
	}
	return copied
}

// mergeRequestDays adds the counts of from to into
func mergeRequestDays(into, from map[string]map[requestRow]*requestCounts) {
	for day, rows := range from {
		if into[day] == nil {
			into[day] = map[requestRow]*requestCounts{}
		}
		for row, counts := range rows {
			if into[day][row] == nil {
				into[day][row] = &requestCounts{}
			}
			into[day][row].merge(counts)
		}
	}
}

// requestRoute returns the pattern of the route the request is for, such as
// /api/build/{id}, and "" if none matches. It finds it itself, because the
// auth middleware refuses requests before chi has matched them. Call it in a
// middleware of the router, which puts the route context in the request.
func requestRoute(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil || rctx.Routes == nil {
		return ""
	}
	routePath := r.URL.RawPath
	if routePath == "" {
		routePath = r.URL.Path
	}
	return rctx.Routes.Find(chi.NewRouteContext(), r.Method, routePath)
}

// requestVia says how a request came in, by the credentials it carried
func requestVia(r *http.Request) string {
	if r.Context().Value(mcpCallKey{}) != nil {
		return ViaMCP
	}
	if token, ok := apiKeyToken(r); ok {
		switch {
		case r.URL.Path == mcpRoute:
			return ViaMCP
		case strings.HasPrefix(token, buildTokenPrefix):
			return ViaBuild
		}
		return ViaKey
	}
	if _, _, ok := r.BasicAuth(); ok {
		return ViaBasic
	}
	if _, err := sessionCookieValue(r); err == nil {
		return ViaWeb
	}
	return ViaNone
}

// spaRoute serves the web UI's files and pages, which aren't counted
const spaRoute = "/*"

// waitsForBuild says whether the request waited for a build to end, so its
// time says how long the build took, not wakeci
func waitsForBuild(query url.Values) bool {
	wait := query.Get("wait")
	return wait != "" && wait != "0"
}

// countRequest counts a request that LogMi saw end with status after took.
// The calls an MCP tool makes aren't counted: the tool call is.
// requestWho is who made a request, as its rows count it: a build's token
// as builds, not each build, which would give every build rows of its own
// that the usage page never shows apart
func requestWho(r *http.Request) buildTrigger {
	who := triggerOf(r)
	who.Build = 0
	return who
}

func countRequest(r *http.Request, route, via string, status int, started time.Time, took time.Duration) {
	if r.Context().Value(mcpCallKey{}) != nil || route == spaRoute {
		return
	}
	if route != "" {
		route = r.Method + " " + route
	}
	row := requestRow{Via: via, Route: route, Outcome: statusOutcome(status), Who: requestWho(r)}
	timed := route != "GET /ws" && !waitsForBuild(r.URL.Query())
	requestStats.add(usageDay(started), row, took, timed)
}

// countToolCall counts an MCP tool call, made in the request parent
func countToolCall(parent *http.Request, tool, outcome string, started time.Time, took time.Duration, timed bool) {
	row := requestRow{Via: ViaMCP, Tool: tool, Outcome: outcome, Who: requestWho(parent)}
	requestStats.add(usageDay(started), row, took, timed)
}

// requestDayRecord is a day's counts as stored in RequestStatsBucket, by the
// day such as 2026-11-24
type requestDayRecord struct {
	Rows []requestDayRow `json:"rows"`
}

type requestDayRow struct {
	requestRow
	requestCounts
}

func decodeRequestDay(data []byte) (map[requestRow]*requestCounts, error) {
	rows := map[requestRow]*requestCounts{}
	if data == nil {
		return rows, nil
	}
	var record requestDayRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return rows, err
	}
	for i := range record.Rows {
		row := record.Rows[i]
		if rows[row.requestRow] == nil {
			rows[row.requestRow] = &requestCounts{}
		}
		rows[row.requestRow].merge(&row.requestCounts)
	}
	return rows, nil
}

func encodeRequestDay(rows map[requestRow]*requestCounts) ([]byte, error) {
	record := requestDayRecord{Rows: make([]requestDayRow, 0, len(rows))}
	for row, counts := range rows {
		record.Rows = append(record.Rows, requestDayRow{requestRow: row, requestCounts: *counts})
	}
	slices.SortFunc(record.Rows, func(a, b requestDayRow) int {
		return cmp.Or(cmp.Compare(a.Via, b.Via), cmp.Compare(a.Route, b.Route), cmp.Compare(a.Tool, b.Tool),
			cmp.Compare(a.Outcome, b.Outcome), cmp.Compare(a.Who.Kind, b.Who.Kind), cmp.Compare(a.Who.User, b.Who.User),
			cmp.Compare(a.Who.APIKey, b.Who.APIKey))
	})
	return json.Marshal(record)
}

// flushRequestStats saves the counts not saved yet
func flushRequestStats() error {
	pending := requestStats.takeAll()
	if len(pending) == 0 {
		return nil
	}
	err := DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(RequestStatsBucket)
		if b == nil {
			return nil
		}
		for day, rows := range pending {
			saved, err := decodeRequestDay(b.Get([]byte(day)))
			if err != nil {
				// Keeping it would stop every later save of the day
				L.Error("decode request stats; replacing them", "day", day, "err", err)
			}
			mergeRequestDays(map[string]map[requestRow]*requestCounts{day: saved}, map[string]map[requestRow]*requestCounts{day: rows})
			data, err := encodeRequestDay(saved)
			if err != nil {
				return err
			}
			if err := b.Put([]byte(day), data); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		requestStats.putBack(pending)
	}
	return err
}

// startRequestStatsFlusher saves the counts every interval until stop is
// called; stop waits for a save in progress
func startRequestStatsFlusher(interval time.Duration) (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := flushRequestStats(); err != nil {
					L.Error("save request stats", "err", err)
				}
			}
		}
	})
	return func() {
		close(done)
		wg.Wait()
	}
}

// pruneRequestStats deletes the days older than requestStatsRetention
func pruneRequestStats(tx *bolt.Tx, now time.Time, logger *slog.Logger) error {
	b := tx.Bucket(RequestStatsBucket)
	if b == nil {
		return nil
	}
	cutoff := []byte(usageDay(now.Add(-requestStatsRetention)))
	c := b.Cursor()
	deleted := 0
	for k, _ := c.First(); k != nil && string(k) < string(cutoff); k, _ = c.First() {
		if err := c.Delete(); err != nil {
			return err
		}
		deleted++
	}
	if deleted > 0 {
		logger.Info("deleted old request stats", "days", deleted)
	}
	return nil
}

// RequestStatsData is how wakeci was used over a range of days
type RequestStatsData struct {
	// Days is how many days the range has, the last one today
	Days int `json:"days"`
	// From is the range's first day and Today its last, such as 2026-11-12,
	// in the server's time zone
	From  string `json:"from"`
	Today string `json:"today"`
	// Since is the first day wakeci counted requests on, null if none.
	// Days before it have no data
	Since *string `json:"since" extensions:"x-nullable"`
	// Totals are the requests and tool calls in the range
	Totals RequestTotals `json:"totals"`
	// ByDay has every day of the range, oldest first
	ByDay []RequestDay `json:"byDay"`
	// Interfaces are the requests by how they came in, whatever via asked for
	Interfaces []InterfaceUse `json:"interfaces"`
	// People are the requests by who made them, whatever who asked for;
	// requests whose credentials were missing or wrong have no one
	People []PersonUse `json:"people"`
	// Routes are the requests by how they came in and their route, most
	// first; an empty route is a path no route matches (for API keys, also
	// routes keys can't use)
	Routes []RouteUse `json:"routes"`
	// Tools are the MCP tool calls by tool, most first
	Tools []ToolUse `json:"tools"`
}

// RequestOutcomes count requests or tool calls by outcome
type RequestOutcomes struct {
	// Success is a status below 400
	Success int64 `json:"success"`
	// Refused is 401, 403 or 429: wrong credentials, no permission or too
	// many requests
	Refused int64 `json:"refused"`
	// ClientError is any other 4xx, such as 400 or 404
	ClientError int64 `json:"clientError"`
	// ServerError is 5xx
	ServerError int64 `json:"serverError"`
}

func (o *RequestOutcomes) add(outcome string, n int64) {
	switch outcome {
	case OutcomeSuccess:
		o.Success += n
	case OutcomeRefused:
		o.Refused += n
	case OutcomeClientError:
		o.ClientError += n
	default:
		o.ServerError += n
	}
}

// RequestTime is how long requests took, estimated from time buckets, which
// makes p50 and p95 accurate to within about a third; Max is exact. Only
// requests whose time counts are in it: not WebSockets, nor reads that waited
// for a build to end.
type RequestTime struct {
	// Count is how many requests the times come from
	Count int64         `json:"count"`
	P50   time.Duration `json:"p50" swaggertype:"integer"`
	P95   time.Duration `json:"p95" swaggertype:"integer"`
	Max   time.Duration `json:"max" swaggertype:"integer"`
}

func requestTimeOf(c *requestCounts) RequestTime {
	return RequestTime{Count: c.Timed, P50: c.percentile(0.5), P95: c.percentile(0.95), Max: c.Max}
}

// RequestTotals are the HTTP requests and MCP tool calls of a range. An MCP
// tool call is also one request to /mcp.
type RequestTotals struct {
	Requests int64 `json:"requests"`
	RequestOutcomes
	ToolCalls    int64           `json:"toolCalls"`
	ToolOutcomes RequestOutcomes `json:"toolOutcomes"`
}

// RequestDay is a day's requests and MCP tool calls
type RequestDay struct {
	Day      string `json:"day"`
	Requests int64  `json:"requests"`
	RequestOutcomes
	ToolCalls int64 `json:"toolCalls"`
}

// InterfaceUse is the requests that came in one way
type InterfaceUse struct {
	// Via is web (a session: the web UI), basic (Basic auth with the
	// password), key (an API key on a REST route), mcp (an API key on /mcp)
	// or none (no credentials)
	Via      string `json:"via"`
	Requests int64  `json:"requests"`
	RequestOutcomes
}

// PersonUse is the requests of the admin password, a single sign-on user or
// an API key
type PersonUse struct {
	// ID names them in the who parameter
	ID string `json:"id"`
	// Kind is password, sso or apikey
	Kind string `json:"kind"`
	// User is the single sign-on email, or who created the API key (empty
	// for the admin password)
	User string `json:"user"`
	// APIKey is the key's ID and KeyName its name, empty once revoked
	APIKey   string `json:"apiKey,omitempty"`
	KeyName  string `json:"keyName,omitempty"`
	Requests int64  `json:"requests"`
	RequestOutcomes
	// ByVia counts the requests by how they came in
	ByVia     map[string]int64 `json:"byVia"`
	ToolCalls int64            `json:"toolCalls"`
}

// RouteUse is the requests to a route that came in one way
type RouteUse struct {
	Via string `json:"via"`
	// Route is the method and pattern, such as GET /api/build/{id}; empty for
	// paths no route matches
	Route    string `json:"route"`
	Requests int64  `json:"requests"`
	RequestOutcomes
	Time RequestTime `json:"time"`
}

// ToolUse is the calls of an MCP tool
type ToolUse struct {
	Tool  string `json:"tool"`
	Calls int64  `json:"calls"`
	RequestOutcomes
	Time RequestTime `json:"time"`
}

type requestStatsQuery struct {
	days int
	// via and who, if set, keep only the requests that came in that way and
	// of that person (PersonUse.ID)
	via string
	who string
}

func requestStatsQueryFrom(values url.Values) (requestStatsQuery, error) {
	q := requestStatsQuery{days: defaultAnalyticsDays, via: values.Get("via"), who: values.Get("who")}
	if s := values.Get("days"); s != "" {
		days, err := strconv.Atoi(s)
		if err != nil || days < 1 || days > maxAnalyticsDays {
			return q, fmt.Errorf("days must be a number from 1 to %d", maxAnalyticsDays)
		}
		q.days = days
	}
	if q.via != "" && !slices.Contains(viaKinds, q.via) {
		return q, fmt.Errorf("via must be one of %v", viaKinds)
	}
	return q, nil
}

// personID names who made a request in the who parameter, "" for no one
func personID(who buildTrigger) string {
	switch who.Kind {
	case TriggerPassword:
		return TriggerPassword
	case TriggerSSO:
		return TriggerSSO + ":" + who.User
	case TriggerAPIKey:
		return TriggerAPIKey + ":" + who.APIKey
	}
	return ""
}

// readRequestDays returns the saved counts of the days from first to last,
// and the first day wakeci has counts of, "" if none
func readRequestDays(tx *bolt.Tx, first, last string) (map[string]map[requestRow]*requestCounts, string) {
	days := map[string]map[requestRow]*requestCounts{}
	b := tx.Bucket(RequestStatsBucket)
	if b == nil {
		return days, ""
	}
	c := b.Cursor()
	since, _ := c.First()
	for k, v := c.Seek([]byte(first)); k != nil && string(k) <= last; k, v = c.Next() {
		rows, err := decodeRequestDay(v)
		if err != nil {
			L.Error("decode request stats", "day", string(k), "err", err)
			continue
		}
		days[string(k)] = rows
	}
	return days, string(since)
}

// computeRequestStats sums up the counts of the range's days
func computeRequestStats(days map[string]map[requestRow]*requestCounts, q requestStatsQuery, now time.Time, since string) *RequestStatsData {
	r := rangeOf(analyticsQuery{days: q.days}, now)
	data := &RequestStatsData{
		Days:       q.days,
		From:       r.from.Format(time.DateOnly),
		Today:      usageDay(now),
		ByDay:      []RequestDay{},
		Interfaces: []InterfaceUse{},
		People:     []PersonUse{},
		Routes:     []RouteUse{},
		Tools:      []ToolUse{},
	}
	if since != "" {
		data.Since = &since
	}
	type routeKey struct{ via, route string }
	interfaces := map[string]*InterfaceUse{}
	people := map[string]*PersonUse{}
	routes := map[routeKey]*RouteUse{}
	routeCounts := map[routeKey]*requestCounts{}
	tools := map[string]*ToolUse{}
	toolCounts := map[string]*requestCounts{}

	for day := r.from; day.Before(r.end); day = day.AddDate(0, 0, 1) {
		name := day.Format(time.DateOnly)
		dayData := RequestDay{Day: name}
		for row, counts := range days[name] {
			id := personID(row.Who)
			viaMatches := q.via == "" || row.Via == q.via
			whoMatches := q.who == "" || id == q.who
			// Each facet keeps the choices of the other filter
			if whoMatches && row.Tool == "" {
				use := interfaces[row.Via]
				if use == nil {
					use = &InterfaceUse{Via: row.Via}
					interfaces[row.Via] = use
				}
				use.Requests += counts.N
				use.add(row.Outcome, counts.N)
			}
			if viaMatches && id != "" {
				person := people[id]
				if person == nil {
					person = &PersonUse{ID: id, Kind: row.Who.Kind, User: row.Who.User, APIKey: row.Who.APIKey, ByVia: map[string]int64{}}
					people[id] = person
				}
				if row.Tool != "" {
					person.ToolCalls += counts.N
				} else {
					person.Requests += counts.N
					person.add(row.Outcome, counts.N)
					person.ByVia[row.Via] += counts.N
				}
			}
			if !viaMatches || !whoMatches {
				continue
			}
			if row.Tool != "" {
				dayData.ToolCalls += counts.N
				data.Totals.ToolCalls += counts.N
				data.Totals.ToolOutcomes.add(row.Outcome, counts.N)
				tool := tools[row.Tool]
				if tool == nil {
					tool = &ToolUse{Tool: row.Tool}
					tools[row.Tool] = tool
					toolCounts[row.Tool] = &requestCounts{}
				}
				tool.Calls += counts.N
				tool.add(row.Outcome, counts.N)
				toolCounts[row.Tool].merge(counts)
				continue
			}
			dayData.Requests += counts.N
			dayData.add(row.Outcome, counts.N)
			data.Totals.Requests += counts.N
			data.Totals.add(row.Outcome, counts.N)
			key := routeKey{row.Via, row.Route}
			route := routes[key]
			if route == nil {
				route = &RouteUse{Via: row.Via, Route: row.Route}
				routes[key] = route
				routeCounts[key] = &requestCounts{}
			}
			route.Requests += counts.N
			route.add(row.Outcome, counts.N)
			routeCounts[key].merge(counts)
		}
		data.ByDay = append(data.ByDay, dayData)
	}

	for _, use := range interfaces {
		data.Interfaces = append(data.Interfaces, *use)
	}
	slices.SortFunc(data.Interfaces, func(a, b InterfaceUse) int {
		return cmp.Or(cmp.Compare(b.Requests, a.Requests), cmp.Compare(a.Via, b.Via))
	})
	for _, person := range people {
		data.People = append(data.People, *person)
	}
	slices.SortFunc(data.People, func(a, b PersonUse) int {
		return cmp.Or(cmp.Compare(b.Requests+b.ToolCalls, a.Requests+a.ToolCalls), cmp.Compare(a.ID, b.ID))
	})
	for key, route := range routes {
		route.Time = requestTimeOf(routeCounts[key])
		data.Routes = append(data.Routes, *route)
	}
	slices.SortFunc(data.Routes, func(a, b RouteUse) int {
		return cmp.Or(cmp.Compare(b.Requests, a.Requests), cmp.Compare(a.Route, b.Route), cmp.Compare(a.Via, b.Via))
	})
	for _, name := range slices.Sorted(maps.Keys(tools)) {
		tool := tools[name]
		tool.Time = requestTimeOf(toolCounts[name])
		data.Tools = append(data.Tools, *tool)
	}
	slices.SortStableFunc(data.Tools, func(a, b ToolUse) int { return cmp.Compare(b.Calls, a.Calls) })
	return data
}

// addPersonKeyNames names the API keys of people that still exist
func addPersonKeyNames(tx *bolt.Tx, people []PersonUse) {
	b := tx.Bucket(APIKeysBucket)
	if b == nil {
		return
	}
	for i := range people {
		if people[i].APIKey == "" {
			continue
		}
		var key storedAPIKey
		if data := b.Get([]byte(people[i].APIKey)); data != nil && json.Unmarshal(data, &key) == nil {
			people[i].KeyName = key.Name
		}
	}
}

// HandleRequestStats returns how wakeci was used
// @Summary      How wakeci was used: requests and MCP tool calls over a range of days
// @Description  Needs write: the admin password, or a single sign-on user whose groups allow write, who can read the database anyway. Not available with an API key. HTTP requests by day, how they came in (via: web for the web UI's session, basic for Basic auth with the password, key for an API key on a REST route, mcp for an API key on /mcp, none for no credentials), who made them, route and outcome, with the median, 95th percentile and slowest time of each route; and MCP tool calls the same way. An MCP tool call is also one request to /mcp; the API calls a tool makes aren't counted again. Outcomes: success below 400, refused 401, 403 or 429, clientError other 4xx, serverError 5xx. Times are estimated from buckets and leave out WebSockets and reads that waited for a build (wait). Days are in the server's time zone; the counts of the last minute are included.
// @Tags         analytics
// @Produce      json
// @Param        days  query   integer   false  "How many days, the last one today, 1 to 365; 30 if not set"
// @Param        via   query   string    false  "Only requests that came in this way: web, basic, key, mcp, build (a build's token, as wake sends it) or none"
// @Param        who   query   string    false  "Only requests of this person or key: an id from people, such as password, sso:ann@example.com or apikey:a1b2c3"
// @Success      200   {object}  RequestStatsData
// @Success      304   {string}  string
// @Failure      400   {string}  string
// @Failure      401   {string}  string
// @Failure      403   {string}  string
// @Failure      429   {string}  string
// @Failure      500   {string}  string
// @Router       /api/analytics/requests [get]
func HandleRequestStats(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	q, err := requestStatsQueryFrom(r.URL.Query())
	if err != nil {
		logger.Warn("invalid request stats request", "err", err)
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now()
	first := rangeOf(analyticsQuery{days: q.days}, now).from.Format(time.DateOnly)
	last := usageDay(now)
	var data *RequestStatsData
	err = DB.View(func(tx *bolt.Tx) error {
		days, since := readRequestDays(tx, first, last)
		pending := requestStats.copyDays(first, last)
		mergeRequestDays(days, pending)
		if since == "" && len(pending) > 0 {
			since = slices.Min(slices.Collect(maps.Keys(pending)))
		}
		data = computeRequestStats(days, q, now, since)
		addPersonKeyNames(tx, data.People)
		return nil
	})
	if err != nil {
		logger.Error("read request stats", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	writeCachedJSON(w, r, logger, data)
}

// requestLineAttrs are the request line's attributes of what request stats
// count: how the request came in, its route, and the MCP tool that a request
// to /mcp called or whose call made the request
func requestLineAttrs(r *http.Request, info *requestInfo, route, via string) []any {
	attrs := []any{"via", via}
	if route != "" {
		attrs = append(attrs, "route", route)
	}
	tool := info.mcpTool
	if name, ok := r.Context().Value(mcpCallKey{}).(string); ok {
		tool = name
	}
	if tool != "" {
		attrs = append(attrs, "tool", tool)
	}
	return attrs
}
