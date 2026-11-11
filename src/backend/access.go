package main

import (
	"fmt"
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
