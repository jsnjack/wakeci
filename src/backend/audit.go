package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// auditLogger writes the audit log: one line per sign-in and per change,
// saying who did what, from where and whether it worked. It is a record for
// auditors, not diagnostics, so it is always on, whatever --debug says.
// initAuditLogger points it at stderr; until then (e.g. in tests) it discards.
var auditLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// initAuditLogger sends audit events to w, each naming this server so the
// logs of several servers can be read together
func initAuditLogger(w io.Writer, server string) {
	auditLogger = slog.New(slog.NewTextHandler(w, nil)).With("server", server)
}

// auditServerName names this server in audit events: the configured
// hostname, or the machine's
func auditServerName(hostname string) string {
	if hostname != "" {
		return hostname
	}
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return name
}

// maxForwardedForLength bounds the X-Forwarded-For header in the logs.
// A proxy appends the address it saw to the end, after whatever the client
// sent, so a longer header keeps its start and its end.
const (
	maxForwardedForLength = 256
	forwardedForStart     = 64
)

// maxAuditValueLength bounds each value in the audit log, so a request can't
// write a huge line, e.g. with a long parameter name
const maxAuditValueLength = 1024

// requestInfo is what the request line and the audit log know about a
// request. LogMi creates it, the auth middleware records who made the
// request once it has checked the credentials, and handlers add what they
// did to the request's audit event.
type requestInfo struct {
	logID string
	// ip is the address the connection comes from
	ip string
	// forwardedFor is the X-Forwarded-For header as sent. Anyone can set it,
	// so it is logged next to ip, never instead of it.
	forwardedFor string
	// auth is how the request was authenticated: password (a session from
	// the password), sso, basic (Basic auth with the password) or apikey
	auth string
	// user is the SSO email; for an API key, who created it
	user string
	// apiKey is the ID of the API key the request carries
	apiKey string
	// attrs are added by the handler to the request's audit event
	attrs []any
}

type requestInfoKey struct{}

// withRequestInfo returns the request's info, adding new info to the
// request if it has none yet
func withRequestInfo(r *http.Request) (*http.Request, *requestInfo) {
	if info, ok := r.Context().Value(requestInfoKey{}).(*requestInfo); ok {
		return r, info
	}
	info := &requestInfo{
		logID:        GenerateRandomString(5),
		ip:           clientIP(r),
		forwardedFor: forwardedFor(r),
	}
	return r.WithContext(context.WithValue(r.Context(), requestInfoKey{}, info)), info
}

// requestInfoOf returns the request's info. A request that has none gets
// info that isn't kept, so callers never need to check.
func requestInfoOf(r *http.Request) *requestInfo {
	_, info := withRequestInfo(r)
	return info
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func forwardedFor(r *http.Request) string {
	value := strings.Join(r.Header.Values("X-Forwarded-For"), ", ")
	if len(value) > maxForwardedForLength {
		value = value[:forwardedForStart] + "..." + value[len(value)-(maxForwardedForLength-forwardedForStart):]
	}
	return value
}

// boundAuditValues cuts string values longer than maxAuditValueLength,
// marking the cut with "..."
func boundAuditValues(attrs []any) []any {
	for i, attr := range attrs {
		if value, ok := attr.(string); ok && len(value) > maxAuditValueLength {
			attrs[i] = value[:maxAuditValueLength] + "..."
		}
	}
	return attrs
}

// setRequestUser records who made the request, for its request line and
// audit event. Only call it once the credentials have been checked.
func setRequestUser(r *http.Request, auth, user, apiKey string) {
	info := requestInfoOf(r)
	info.auth, info.user, info.apiKey = auth, user, apiKey
}

// sessionAuth is how a session was created: an SSO login, or the password
// for a session without a user
func sessionAuth(user string) string {
	if user == "" {
		return "password"
	}
	return "sso"
}

// addAuditAttrs adds details of what the handler did, e.g. the build it
// created, to the request's audit event
func addAuditAttrs(r *http.Request, attrs ...any) {
	info := requestInfoOf(r)
	info.attrs = append(info.attrs, attrs...)
}

// who returns the attrs that say who made the request
func (info *requestInfo) who() []any {
	var attrs []any
	if info.auth != "" {
		attrs = append(attrs, "auth", info.auth)
	}
	if info.user != "" {
		attrs = append(attrs, "user", info.user)
	}
	if info.apiKey != "" {
		attrs = append(attrs, "apiKey", info.apiKey)
	}
	return attrs
}

// from returns the attrs that say where the request came from
func (info *requestInfo) from() []any {
	attrs := []any{"ip", info.ip}
	if info.forwardedFor != "" {
		attrs = append(attrs, "forwardedFor", info.forwardedFor)
	}
	return attrs
}

// audit writes an audit event about request r: what happened, who did it
// (once the auth middleware knows), any attrs, and where it came from
func audit(r *http.Request, event, outcome string, attrs ...any) {
	info := requestInfoOf(r)
	all := []any{"event", event, "outcome", outcome}
	all = append(all, info.who()...)
	all = append(all, attrs...)
	all = append(all, info.from()...)
	all = append(all, "logID", info.logID)
	auditLogger.Info("audit", boundAuditValues(all)...)
}

// auditScheduled writes an audit event that no request caused, e.g. a build
// started by its job's interval
func auditScheduled(event, outcome string, attrs ...any) {
	auditLogger.Info("audit", boundAuditValues(append([]any{"event", event, "outcome", outcome}, attrs...))...)
}

// blockedAttrs marks the failed attempt that blocked the client
func blockedAttrs(blocked bool) []any {
	if blocked {
		return []any{"blocked", true}
	}
	return nil
}

// auditedRoute names the audit event of a route that changes something,
// and finds what it acts on from the URL, so a request refused before the
// handler still names it
type auditedRoute struct {
	event string
	attrs func(r *http.Request, param func(name string) string) []any
}

// auditedRoutes are the routes that change something, by method and chi
// route pattern. TestAuditedRoutes fails for a route missing here, so a new
// route can't go unaudited.
var auditedRoutes = map[string]auditedRoute{
	"POST /api/jobs/create":           {event: "job.create"},
	"POST /api/job/{name}/run":        {event: "build.run", attrs: jobOfURL},
	"POST /api/job/{name}":            {event: "job.edit", attrs: jobOfURL},
	"DELETE /api/job/{name}":          {event: "job.delete", attrs: jobOfURL},
	"POST /api/job/{name}/set_active": {event: "job.set_active", attrs: jobOfURL},
	"POST /api/build/{id}/abort":      {event: "build.abort", attrs: buildOfURL},
	"POST /api/build/{id}/start":      {event: "build.start", attrs: buildOfURL},
	"POST /api/build/{id}/flush":      {event: "build.flush", attrs: buildOfURL},
	"POST /api/settings":              {event: "settings.change"},
	"POST /api/keys/":                 {event: "key.create"},
	"DELETE /api/keys/{id}":           {event: "key.revoke", attrs: keyOfURL},
	"DELETE /api/keys/":               {event: "access.revoke", attrs: personOfURL},
}

func jobOfURL(_ *http.Request, param func(string) string) []any {
	return []any{"job", param("name")}
}

func buildOfURL(_ *http.Request, param func(string) string) []any {
	attrs := []any{"build", param("id")}
	if id, err := strconv.Atoi(param("id")); err == nil {
		if job, _, err := buildJob(id); err == nil {
			attrs = append(attrs, "job", job)
		}
	}
	return attrs
}

func keyOfURL(_ *http.Request, param func(string) string) []any {
	return []any{"key", param("id")}
}

func personOfURL(r *http.Request, _ func(string) string) []any {
	// Trimmed like HandleAPIKeysRevokeBy, so it names the account revoked
	return []any{"person", strings.TrimSpace(r.URL.Query().Get("createdBy"))}
}

// auditMux matches a request to its audited route. AuditMi runs before the
// auth middleware, which refuses requests before the router has matched
// them, so it can't read the route from the router.
var auditMux = newAuditMux()

func newAuditMux() *chi.Mux {
	mux := chi.NewRouter()
	for route := range auditedRoutes {
		method, pattern, _ := strings.Cut(route, " ")
		mux.Method(method, pattern, http.NotFoundHandler())
	}
	return mux
}

// findAuditedRoute returns the audited route of a request, and a function
// that returns its URL parameters
func findAuditedRoute(r *http.Request) (auditedRoute, func(string) string, bool) {
	// The path chi routes by
	path := r.URL.RawPath
	if path == "" {
		path = r.URL.Path
	}
	rctx := chi.NewRouteContext()
	pattern := auditMux.Find(rctx, r.Method, path)
	// A route ending in "/" is the root of a mounted router, which chi also
	// serves without the slash: POST /api/keys creates a key
	if pattern == "" && !strings.HasSuffix(path, "/") {
		rctx.Reset()
		if found := auditMux.Find(rctx, r.Method, path+"/"); strings.HasSuffix(found, "/") {
			pattern = found
		}
	}
	route, ok := auditedRoutes[r.Method+" "+pattern]
	param := func(name string) string {
		value := rctx.URLParam(name)
		if r.URL.RawPath != "" {
			if unescaped, err := url.PathUnescape(value); err == nil {
				return unescaped
			}
		}
		return value
	}
	return route, param, ok
}

// isReadOnly reports whether a request can't change anything
func isReadOnly(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}

// AuditMi writes an audit event for every request that can change
// something: what it did, who did it and whether it worked. It runs before
// the auth middleware, so refused requests are recorded too.
func AuditMi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isReadOnly(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		r, info := withRequestInfo(r)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		event := "request"
		var attrs []any
		if route, param, ok := findAuditedRoute(r); ok {
			event = route.event
			if route.attrs != nil {
				attrs = route.attrs(r, param)
			}
		}
		attrs = append(attrs, info.attrs...)
		attrs = append(attrs, "status", rec.status, "method", r.Method, "path", r.URL.Path)
		outcome := "ok"
		if rec.status >= http.StatusBadRequest {
			outcome = "failed"
		}
		audit(r, event, outcome, attrs...)
	})
}
