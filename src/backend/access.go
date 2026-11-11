package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// Grant is a set of scopes on a set of jobs: what an API key may do
type Grant struct {
	Scopes []string `json:"scopes"`
	// AllJobs covers every job, including jobs added later. Otherwise the
	// grant covers Jobs by exact name and the jobs that Patterns match.
	AllJobs  bool     `json:"allJobs"`
	Jobs     []string `json:"jobs"`
	Patterns []string `json:"patterns"`
}

// HasScope reports whether the grant includes scope. Write lets its holder
// run any command on the server, so it only counts on a grant that covers
// all jobs.
func (g *Grant) HasScope(scope string) bool {
	return slices.Contains(g.Scopes, scope) && (scope != ScopeWrite || g.AllJobs)
}

// CoversJob reports whether the grant applies to the job name
func (g *Grant) CoversJob(name string) bool {
	return g.AllJobs || slices.Contains(g.Jobs, name) ||
		slices.ContainsFunc(g.Patterns, func(pattern string) bool {
			return matchJobPattern(pattern, name)
		})
}

// Access is what a request may do. The admin password may do everything;
// anyone else may do what one of their grants allows.
type Access struct {
	// Full is the admin password's access
	Full bool `json:"full"`
	// Grants are united: a scope on a job is allowed if any grant allows it
	Grants []Grant `json:"grants"`
	// Within, if set, also has to allow it: an API key made by a member of a
	// single sign-on group can't do more than the creator's groups allow now
	Within *Access `json:"-"`
	// subject names the holder in refusals, e.g. "This API key"
	subject string
	// groups are the single sign-on groups the access comes from
	groups []string
}

// fullAccess is the admin password's access
var fullAccess = &Access{Full: true, Grants: []Grant{}}

// HasScope reports whether the access includes scope on some job
func (a *Access) HasScope(scope string) bool {
	if a.Full {
		return true
	}
	ok := slices.ContainsFunc(a.Grants, func(g Grant) bool { return g.HasScope(scope) })
	return ok && (a.Within == nil || a.Within.HasScope(scope))
}

// Allows reports whether the access includes scope on the job name
func (a *Access) Allows(scope, name string) bool {
	if a.Full {
		return true
	}
	ok := slices.ContainsFunc(a.Grants, func(g Grant) bool { return g.HasScope(scope) && g.CoversJob(name) })
	return ok && (a.Within == nil || a.Within.Allows(scope, name))
}

// checkWithin returns an error if grant has a scope the access lacks, or a
// scope on a job the access doesn't cover. Patterns and all jobs are left to
// Within, which limits them to the access on every request.
func (a *Access) checkWithin(grant *Grant) error {
	if a.Full {
		return nil
	}
	for _, scope := range grant.Scopes {
		if !a.HasScope(scope) {
			return fmt.Errorf("your groups don't have the %s scope", scope)
		}
		for _, job := range grant.Jobs {
			if !a.Allows(scope, job) {
				return fmt.Errorf("your groups don't allow %s on %s", scope, job)
			}
		}
	}
	return nil
}

// Reads reports whether a list may show the job name. A request without
// access, i.e. a handler called without its route's middleware, isn't
// filtered.
func (a *Access) Reads(name string) bool {
	return a == nil || a.Allows(ScopeRead, name)
}

// Empty reports whether the access allows nothing at all
func (a *Access) Empty() bool {
	return !a.Full && !slices.ContainsFunc(apiKeyScopes, a.HasScope)
}

type accessContextKey struct{}

// withAccess adds what the request may do to it
func withAccess(r *http.Request, access *Access) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), accessContextKey{}, access))
}

// accessFromContext returns what the request may do, nil if no
// authentication middleware set it
func accessFromContext(ctx context.Context) *Access {
	access, _ := ctx.Value(accessContextKey{}).(*Access)
	return access
}

// requireAccess lets a request through if its access includes scope and,
// when jobOf is set, includes it on the job the request acts on. A request
// without access is refused, so a route can't be opened by a middleware
// that forgot to set it.
func requireAccess(scope string, jobOf jobResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger := requestLogger(r)
			access := accessFromContext(r.Context())
			if access == nil || !access.HasScope(scope) {
				subject := "This request"
				if access != nil {
					subject = access.subject
				}
				logger.Warn("access lacks scope", "scope", scope)
				writePlainText(w, logger, http.StatusForbidden, subject+" doesn't have the "+scope+" scope")
				return
			}
			if jobOf != nil && !access.Full {
				job, status, err := jobOf(r)
				if err != nil {
					logger.Warn("find job for access check", "err", err)
					writePlainText(w, logger, status, err.Error())
					return
				}
				if !access.Allows(scope, job) {
					logger.Warn("access doesn't cover job", "job", job, "scope", scope)
					// The job of a build is only known to those who may see it
					writePlainText(w, logger, http.StatusForbidden, access.subject+" doesn't cover that job")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// grantFromForm validates the scope, allJobs, job and pattern fields of a
// form
func grantFromForm(form url.Values) (Grant, error) {
	grant := Grant{Jobs: []string{}, Patterns: []string{}}
	for _, scope := range form["scope"] {
		if !slices.Contains(apiKeyScopes, scope) {
			return Grant{}, fmt.Errorf("unknown scope %q", scope)
		}
	}
	for _, scope := range apiKeyScopes {
		if slices.Contains(form["scope"], scope) {
			grant.Scopes = append(grant.Scopes, scope)
		}
	}
	if len(grant.Scopes) == 0 {
		return Grant{}, fmt.Errorf("select at least one scope")
	}

	switch form.Get("allJobs") {
	case "true":
		grant.AllJobs = true
	case "false", "":
	default:
		return Grant{}, fmt.Errorf("allJobs must be true or false")
	}
	if !grant.AllJobs {
		var err error
		if grant.Jobs, err = uniqueJobNames(form["job"]); err != nil {
			return Grant{}, err
		}
		if grant.Patterns, err = uniqueJobNames(form["pattern"]); err != nil {
			return Grant{}, err
		}
		if len(grant.Jobs) == 0 && len(grant.Patterns) == 0 {
			return Grant{}, fmt.Errorf("select a job or enter a pattern")
		}
		if slices.Contains(grant.Scopes, ScopeWrite) {
			return Grant{}, fmt.Errorf("write can run any command on the server, so it needs access to all jobs")
		}
	}
	return grant, nil
}

// uniqueJobNames checks job names or patterns and drops repeats
func uniqueJobNames(values []string) ([]string, error) {
	names := []string{}
	for _, value := range values {
		if !isValidJobName(value) {
			return nil, fmt.Errorf("invalid job name or pattern %q", value)
		}
		if !slices.Contains(names, value) {
			names = append(names, value)
		}
	}
	return names, nil
}

// matchJobPattern reports whether name matches pattern, in which * matches
// any run of characters and every other character only itself
func matchJobPattern(pattern, name string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name
	}
	rest, ok := strings.CutPrefix(name, parts[0])
	if !ok {
		return false
	}
	// Taking each middle part at its first match leaves the most room for
	// the ones after it
	for _, part := range parts[1 : len(parts)-1] {
		i := strings.Index(rest, part)
		if i < 0 {
			return false
		}
		rest = rest[i+len(part):]
	}
	return strings.HasSuffix(rest, parts[len(parts)-1])
}
