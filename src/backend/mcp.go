package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpInstructions tells an agent how to use the tools
const mcpInstructions = `wakeci runs CI jobs as builds. Every tool works with the API key of this connection, on the jobs the key covers.
To find out why a build failed, call build_summary with its ID: it lists the failed tasks with the end of their logs, and the summary the job wrote, if any. Then read_log with grep finds more in a log.
A build page URL such as https://ci.example.com/build/123 has the build ID 123. Build IDs belong to one server: whoami says which hostname and range of IDs this server has.
build_summary with wait waits for a running build to end, its on_* and finally tasks included.`

// mcpArtifactLimit is the most of an artifact read_artifact returns
const mcpArtifactLimit = 256 * 1024

// newMCPHandler serves the API as MCP tools for API keys. Each tool calls
// the key router with the request's own key, so it goes through the same
// scope and job checks, audit, limits and errors as the REST routes.
func newMCPHandler(router http.Handler) http.Handler {
	return mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return newMCPServer(&mcpCaller{router: router, parent: r})
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
}

// mcpCaller sends requests to the API on behalf of an MCP request
type mcpCaller struct {
	router http.Handler
	parent *http.Request
}

// bufferedResponse keeps a response in memory
type bufferedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (b *bufferedResponse) Header() http.Header            { return b.header }
func (b *bufferedResponse) Write(data []byte) (int, error) { return b.body.Write(data) }
func (b *bufferedResponse) WriteHeader(status int)         { b.status = status }

// mcpCallKey marks the requests an MCP tool makes, which APIKeyAuthMi
// doesn't count or limit again; only code in wakeci can set it
type mcpCallKey struct{}

// call sends a request to the API with the MCP request's key and client,
// and returns the body of a successful response or the API's error
func (c *mcpCaller) call(ctx context.Context, method, target string, form url.Values) ([]byte, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	// A fresh context that only follows ctx's cancellation: the MCP
	// request's carries its chi route, log ID and audit record, which the
	// API request must not share
	inner, cancel := context.WithCancel(context.WithValue(context.Background(), mcpCallKey{}, true))
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	req, err := http.NewRequestWithContext(inner, method, target, body)
	if err != nil {
		return nil, fmt.Errorf("make request: %w", err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Authorization", c.parent.Header.Get("Authorization"))
	for _, value := range c.parent.Header.Values("X-Forwarded-For") {
		req.Header.Add("X-Forwarded-For", value)
	}
	req.Header.Set("User-Agent", cutLogValue("wakeci-mcp "+c.parent.UserAgent()))
	req.RemoteAddr = c.parent.RemoteAddr
	resp := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
	c.router.ServeHTTP(resp, req)
	if resp.status >= http.StatusBadRequest {
		return nil, fmt.Errorf("%d %s: %s", resp.status, http.StatusText(resp.status), strings.TrimSpace(resp.body.String()))
	}
	return resp.body.Bytes(), nil
}

func (c *mcpCaller) callJSON(ctx context.Context, target string, out any) error {
	data, err := c.call(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s: %w", target, err)
	}
	return nil
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// Tool inputs and outputs

type mcpNoInput struct{}

type mcpListJobsInput struct {
	Search string `json:"search,omitempty" jsonschema:"only jobs whose name contains this text, ignoring case"`
}

type mcpJobs struct {
	Jobs []*JobsListData `json:"jobs"`
}

type mcpFindBuildsInput struct {
	Job    string `json:"job,omitempty" jsonschema:"only builds of this job, by exact name"`
	Status string `json:"status,omitempty" jsonschema:"only builds with this status: pending, running, finished, failed, aborted or timed out"`
	Search string `json:"search,omitempty" jsonschema:"words matched as substrings of the ID, job, status and params; +word must match, -word must not"`
	Limit  int    `json:"limit,omitempty" jsonschema:"how many builds, 1 to 200; 15 if not set"`
	Before int    `json:"before,omitempty" jsonschema:"only builds with a lower ID, to page back"`
	After  int    `json:"after,omitempty" jsonschema:"only builds with a higher ID, to get new ones"`
}

// mcpBuild is a build in a list, without its tasks
type mcpBuild struct {
	ID        int                 `json:"id"`
	Job       string              `json:"job"`
	Status    ItemStatus          `json:"status"`
	StartedAt time.Time           `json:"startedAt"`
	Duration  string              `json:"duration"`
	Params    []map[string]string `json:"params"`
}

type mcpBuilds struct {
	Builds []mcpBuild `json:"builds"`
}

type mcpBuildSummaryInput struct {
	ID   int  `json:"id" jsonschema:"build ID"`
	Tail *int `json:"tail,omitempty" jsonschema:"lines from the end of each failed task's log, 0 to 10000; 100 if not set"`
	Wait int  `json:"wait,omitempty" jsonschema:"wait up to that many seconds, at most 300, for a running build to end"`
}

type mcpReadLogInput struct {
	Build int    `json:"build" jsonschema:"build ID"`
	Task  int    `json:"task" jsonschema:"task ID, from the build's tasks"`
	Tail  int    `json:"tail,omitempty" jsonschema:"only the last lines, 1 to 10000; 200 if neither tail nor grep is set"`
	Grep  string `json:"grep,omitempty" jsonschema:"only lines that contain this text, ignoring case, with their line numbers"`
}

type mcpReadArtifactInput struct {
	Build int    `json:"build" jsonschema:"build ID"`
	Path  string `json:"path" jsonschema:"the artifact's filename, as build_summary lists it"`
}

type mcpAnalyticsInput struct {
	Days    int    `json:"days,omitempty" jsonschema:"how many days, the last one today, 1 to 365; 30 if not set"`
	Job     string `json:"job,omitempty" jsonschema:"only builds of this job, by exact name"`
	Trigger string `json:"trigger,omitempty" jsonschema:"only builds started by: interval, password, sso, apikey or unknown"`
}

type mcpJobInsightsInput struct {
	Job    string `json:"job" jsonschema:"job name"`
	Builds int    `json:"builds,omitempty" jsonschema:"how many of the job's last builds, 1 to 500; 40 if not set"`
}

type mcpRunJobInput struct {
	Name   string            `json:"name" jsonschema:"job name"`
	Params map[string]string `json:"params,omitempty" jsonschema:"the job's parameters to set, by name; the others keep their defaults"`
}

type mcpStartedBuild struct {
	ID   int    `json:"id"`
	Page string `json:"page"`
}

type mcpBuildInput struct {
	ID int `json:"id" jsonschema:"build ID"`
}

// newMCPServer makes the tools for one MCP request
func newMCPServer(c *mcpCaller) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "wakeci", Version: Version}, &mcp.ServerOptions{Instructions: mcpInstructions})
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(server, &mcp.Tool{Name: "whoami", Meta: mcp.Meta{"scope": "any"}, Annotations: readOnly,
		Description: "What this API key may do, and which server answers: its hostname, version and the range of build IDs it has."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpNoInput) (*mcp.CallToolResult, WhoAmIData, error) {
			var data WhoAmIData
			err := c.callJSON(ctx, "/api/whoami", &data)
			return nil, data, err
		})

	mcp.AddTool(server, &mcp.Tool{Name: "list_jobs", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
		Description: "The jobs this key may read, with their descriptions and default parameters."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcpListJobsInput) (*mcp.CallToolResult, mcpJobs, error) {
			var jobs []*JobsListData
			if err := c.callJSON(ctx, "/api/jobs/", &jobs); err != nil {
				return nil, mcpJobs{}, err
			}
			out := mcpJobs{Jobs: []*JobsListData{}}
			for _, job := range jobs {
				if strings.Contains(strings.ToLower(job.Name), strings.ToLower(in.Search)) {
					out.Jobs = append(out.Jobs, job)
				}
			}
			return nil, out, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "find_builds", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
		Description: "Builds, newest first, by job, status or search words."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcpFindBuildsInput) (*mcp.CallToolResult, mcpBuilds, error) {
			query := url.Values{}
			setIf := func(name, value string) {
				if value != "" && value != "0" {
					query.Set(name, value)
				}
			}
			setIf("job", in.Job)
			setIf("status", in.Status)
			setIf("q", in.Search)
			setIf("limit", strconv.Itoa(in.Limit))
			setIf("before", strconv.Itoa(in.Before))
			setIf("after", strconv.Itoa(in.After))
			var builds []*BuildUpdateData
			if err := c.callJSON(ctx, "/api/feed?"+query.Encode(), &builds); err != nil {
				return nil, mcpBuilds{}, err
			}
			out := mcpBuilds{Builds: []mcpBuild{}}
			for _, build := range builds {
				out.Builds = append(out.Builds, mcpBuild{ID: build.ID, Job: build.Name, Status: build.Status,
					StartedAt: build.StartedAt, Duration: build.Duration.Round(time.Second).String(), Params: build.Params})
			}
			return nil, out, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "build_summary", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
		Description: "A build's status, tasks, artifacts, the summary the job wrote, and the end of the log of each task that failed, timed out or was aborted. With wait, it first waits for a running build to end."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcpBuildSummaryInput) (*mcp.CallToolResult, BuildSummary, error) {
			query := url.Values{}
			if in.Tail != nil {
				query.Set("tail", strconv.Itoa(*in.Tail))
			}
			if in.Wait > 0 {
				query.Set("wait", strconv.Itoa(in.Wait))
			}
			var summary BuildSummary
			err := c.callJSON(ctx, "/api/build/"+strconv.Itoa(in.ID)+"/summary?"+query.Encode(), &summary)
			return nil, summary, err
		})

	mcp.AddTool(server, &mcp.Tool{Name: "analytics", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
		Description: "Builds over a range of days: by day and status, success rate, duration, queue wait, and each job's numbers with the jobs failing now. Use it for questions such as which jobs fail most or got slower."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcpAnalyticsInput) (*mcp.CallToolResult, AnalyticsData, error) {
			query := url.Values{}
			if in.Days > 0 {
				query.Set("days", strconv.Itoa(in.Days))
			}
			if in.Job != "" {
				query.Set("job", in.Job)
			}
			if in.Trigger != "" {
				query.Set("trigger", in.Trigger)
			}
			var data AnalyticsData
			err := c.callJSON(ctx, "/api/analytics/?"+query.Encode(), &data)
			return nil, data, err
		})

	mcp.AddTool(server, &mcp.Tool{Name: "job_insights", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
		Description: "A job's last builds in the history with their tasks: each task's runs, failures, durations and change, the task that stopped failed builds, flaky failures, the time to fix a failure, and the success rate of each parameter value."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcpJobInsightsInput) (*mcp.CallToolResult, JobInsights, error) {
			query := url.Values{}
			if in.Builds > 0 {
				query.Set("builds", strconv.Itoa(in.Builds))
			}
			var data JobInsights
			err := c.callJSON(ctx, "/api/job/"+url.PathEscape(in.Job)+"/insights?"+query.Encode(), &data)
			return nil, data, err
		})

	mcp.AddTool(server, &mcp.Tool{Name: "read_log", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
		Description: "Part of a task's log: its last lines, or the lines that contain a text."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcpReadLogInput) (*mcp.CallToolResult, any, error) {
			query := url.Values{}
			if in.Grep != "" {
				query.Set("grep", in.Grep)
			}
			tail := in.Tail
			if tail == 0 && in.Grep == "" {
				tail = 200
			}
			if tail > 0 {
				query.Set("tail", strconv.Itoa(tail))
			}
			data, err := c.call(ctx, http.MethodGet, fmt.Sprintf("/api/build/%d/log/%d?%s", in.Build, in.Task, query.Encode()), nil)
			if err != nil {
				return nil, nil, err
			}
			return textResult(string(data)), nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "read_artifact", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
		Description: "A text artifact of a build, such as a JSON report, up to 256 KB."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcpReadArtifactInput) (*mcp.CallToolResult, any, error) {
			target := storageURL(in.Build, "artifacts/"+strings.TrimPrefix(in.Path, "/"))
			data, err := c.call(ctx, http.MethodGet, target, nil)
			if err != nil {
				return nil, nil, err
			}
			if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
				return nil, nil, fmt.Errorf("%s isn't text; download it from %s", in.Path, target)
			}
			text := string(data)
			if len(text) > mcpArtifactLimit {
				text = strings.ToValidUTF8(text[:mcpArtifactLimit], "") + "\n... (cut at 256 KB)"
			}
			return textResult(text), nil, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "run_job", Meta: mcp.Meta{"scope": "run"}, Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false)},
		Description: "Start a build of a job. Needs the run scope. Parameters can't reference secrets or contain quotes or line breaks."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcpRunJobInput) (*mcp.CallToolResult, mcpStartedBuild, error) {
			form := url.Values{}
			for name, value := range in.Params {
				form.Set(name, value)
			}
			data, err := c.call(ctx, http.MethodPost, "/api/job/"+url.PathEscape(in.Name)+"/run", form)
			if err != nil {
				return nil, mcpStartedBuild{}, err
			}
			id, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				return nil, mcpStartedBuild{}, fmt.Errorf("read the new build's ID from %q: %w", data, err)
			}
			return nil, mcpStartedBuild{ID: id, Page: "/build/" + strconv.Itoa(id)}, nil
		})

	mcp.AddTool(server, &mcp.Tool{Name: "abort_build", Meta: mcp.Meta{"scope": "run"}, Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true)},
		Description: "Abort a pending or running build. Needs the run scope."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcpBuildInput) (*mcp.CallToolResult, any, error) {
			if _, err := c.call(ctx, http.MethodPost, "/api/build/"+strconv.Itoa(in.ID)+"/abort", nil); err != nil {
				return nil, nil, err
			}
			return textResult(fmt.Sprintf("Build %d is being aborted", in.ID)), nil, nil
		})

	return server
}

func ptr[T any](v T) *T {
	return &v
}
