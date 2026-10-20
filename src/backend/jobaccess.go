package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	bolt "go.etcd.io/bbolt"
)

// An API key can be limited to some jobs: then it sees and acts on only the
// jobs whose names match one of its patterns, and their builds, logs and
// artifacts. A pattern is a job name in which * stands for any run of
// characters, so `test_*` also covers test jobs added later.

const (
	maxJobPatterns      = 500
	maxJobPatternLength = 200
)

// matchJobPattern reports whether job name matches pattern. Only * is
// special, unlike path.Match, and job names can't contain it, so any job
// name can be matched literally.
func matchJobPattern(pattern, name string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name
	}
	first, last := parts[0], parts[len(parts)-1]
	if !strings.HasPrefix(name, first) {
		return false
	}
	rest := name[len(first):]
	// Matching each middle part as early as possible leaves the most room
	// for the parts after it
	for _, part := range parts[1 : len(parts)-1] {
		i := strings.Index(rest, part)
		if i < 0 {
			return false
		}
		rest = rest[i+len(part):]
	}
	return strings.HasSuffix(rest, last)
}

// ParseJobPatterns validates job patterns, one per value, and returns them
// deduplicated in order
func ParseJobPatterns(values []string) ([]string, error) {
	var patterns []string
	for _, pattern := range values {
		// A job name, where * may stand in for any part of it
		if len(pattern) > maxJobPatternLength || !isValidJobName(strings.ReplaceAll(pattern, "*", "x")) {
			return nil, fmt.Errorf("invalid job pattern %q", pattern)
		}
		if !slices.Contains(patterns, pattern) {
			patterns = append(patterns, pattern)
		}
	}
	if len(patterns) > maxJobPatterns {
		return nil, fmt.Errorf("a key can have at most %d job patterns", maxJobPatterns)
	}
	return patterns, nil
}

// AllJobs reports whether the principal may access every job
func (p *Principal) AllJobs() bool {
	return len(p.Jobs) == 0
}

// CanAccessJob reports whether the principal may see or act on job name
func (p *Principal) CanAccessJob(name string) bool {
	return p.AllJobs() || slices.ContainsFunc(p.Jobs, func(pattern string) bool {
		return matchJobPattern(pattern, name)
	})
}

// jobFilter returns which jobs a request may see, or nil for all of them.
// Requests only reach handlers through AuthMi and RequireScope, which turn
// away anonymous ones.
func jobFilter(r *http.Request) func(name string) bool {
	p := principalFrom(r)
	if p == nil || p.AllJobs() {
		return nil
	}
	return p.CanAccessJob
}

// RequireJob rejects requests about a job the principal can't access. job
// returns the name of that job. With hide, a refusal looks like a missing
// build: build IDs are guessable, and a 403 naming the job would reveal the
// jobs a key is kept from. It must run after AuthMi.
func RequireJob(job func(r *http.Request) (string, error), hide bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger, ok := r.Context().Value(HL).(*slog.Logger)
			if !ok {
				logger = L
			}
			p := principalFrom(r)
			if p != nil && p.AllJobs() {
				next.ServeHTTP(w, r)
				return
			}
			name, err := job(r)
			if errors.Is(err, errBuildNotFound) {
				writeText(w, logger, http.StatusNotFound, "Build not found")
				return
			}
			if err != nil {
				logger.Error("find the job of a request", "err", err)
				writeText(w, logger, http.StatusInternalServerError, err.Error())
				return
			}
			if p == nil || !p.CanAccessJob(name) {
				logger.Warn("job not allowed", "job", name, "principal", p)
				if hide {
					writeText(w, logger, http.StatusNotFound, "Build not found")
					return
				}
				writeText(w, logger, http.StatusForbidden, fmt.Sprintf("Forbidden: no access to job %q", name))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// jobInPath is the job of /api/job/{name} routes. An invalid name yields "",
// which only a `*` pattern matches; the handler then rejects it.
func jobInPath(r *http.Request) (string, error) {
	name, _ := jobNameParam(r)
	return name, nil
}

// jobInForm is the job a new job would be created as
func jobInForm(r *http.Request) (string, error) {
	return r.FormValue("name"), nil
}

// jobOfBuildInPath is the job of the build in /api/build/{id} routes
func jobOfBuildInPath(r *http.Request) (string, error) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		return "", errBuildNotFound
	}
	return jobOfBuild(id)
}

// jobOfStoredFile is the job of the build a /storage/build/ file belongs to.
// It resolves the path the way HandleWakespaceResource and http.FileServer
// will, so `5/../7/x` counts as build 7's.
func jobOfStoredFile(r *http.Request) (string, error) {
	id, _, err := storedFile(strings.TrimPrefix(r.URL.Path, "/storage/build/"))
	if err != nil {
		return "", err
	}
	return jobOfBuild(id)
}

// jobOfBuild returns the name of the job build id ran, from its history entry,
// which is written as soon as the build is queued
func jobOfBuild(id int) (string, error) {
	var name string
	err := DB.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(HistoryBucket).Get(Itob(id))
		if v == nil {
			return errBuildNotFound
		}
		var build BuildUpdateData
		if err := json.Unmarshal(v, &build); err != nil {
			return fmt.Errorf("unmarshal build %d: %w", id, err)
		}
		name = build.Name
		return nil
	})
	return name, err
}
