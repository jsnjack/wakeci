package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
		return newMCPServer(&mcpCaller{router: router, parent: r, access: accessFromContext(r.Context())})
	}, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
}

// mcpCaller sends requests to the API on behalf of an MCP request
type mcpCaller struct {
	router http.Handler
	parent *http.Request
	// access is what the request's key may do; nil lists every tool, as the
	// help page does
	access *Access
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

// mcpRoute is where the MCP server answers API keys
const mcpRoute = "/mcp"

// mcpCallKey marks the requests an MCP tool makes, which APIKeyAuthMi
// doesn't count or limit again and the request stats don't count; its value
// is the tool's name. Only code in wakeci can set it.
type mcpCallKey struct{}

// mcpToolKey carries the name of the tool being called to its API calls
type mcpToolKey struct{}

// mcpAPIError is an error answer of the API to a tool's call, or a tool's
// own refusal of what it was asked, with the HTTP status it stands for
type mcpAPIError struct {
	status int
	msg    string
}

func (e *mcpAPIError) Error() string { return e.msg }

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
	tool, _ := ctx.Value(mcpToolKey{}).(string)
	inner, cancel := context.WithCancel(context.WithValue(context.Background(), mcpCallKey{}, tool))
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
		return nil, &mcpAPIError{status: resp.status,
			msg: fmt.Sprintf("%d %s: %s", resp.status, http.StatusText(resp.status), strings.TrimSpace(resp.body.String()))}
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
	Grep  string `json:"grep,omitempty" jsonschema:"only lines whose text contains this, ignoring case and the time each line starts with; with their line numbers"`
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
	Parent int               `json:"parent,omitempty" jsonschema:"ID of the pending or running build that starts this one; the new build is shown under it"`
}

type mcpStartedBuild struct {
	ID   int    `json:"id"`
	Page string `json:"page"`
}

type mcpBuildInput struct {
	ID int `json:"id" jsonschema:"build ID"`
}

// waitsForBuild says whether the call waits for the build to end, so that
// its time says how long the build took, not wakeci
func (in mcpBuildSummaryInput) waitsForBuild() bool {
	return in.Wait > 0
}

// handleMCPStream answers a client that asks for a stream of messages from
// the server: this server is stateless and has none, which MCP says to
// answer with 405
func handleMCPStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Allow", http.MethodPost)
	writePlainText(w, requestLogger(r), http.StatusMethodNotAllowed, "This MCP server sends no stream: POST each request to /mcp")
}

// mcpListedScopes are the scopes that have tools; write has none
var mcpListedScopes = []string{ScopeRead, ScopeRun}

// mcpScopeNote tells an agent which scopes its key lacks, since their tools
// aren't listed; "" if it has them all
func mcpScopeNote(access *Access) string {
	var missing []string
	for _, scope := range mcpListedScopes {
		if access != nil && !access.HasScope(scope) {
			missing = append(missing, scope)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return "\nThis API key doesn't have the " + strings.Join(missing, " or ") + " scope, so the tools that need it aren't listed."
}

// addTool adds a tool whose calls are counted in the request stats and named
// in the request line of their request to /mcp. A tool that needs a scope
// the key lacks isn't added, so the key lists only the tools it can use.
func addTool[In, Out any](server *mcp.Server, c *mcpCaller, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, Out]) {
	if scope, _ := tool.Meta["scope"].(string); c.access != nil && scope != "any" && !c.access.HasScope(scope) {
		return
	}
	mcp.AddTool(server, tool, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		started := time.Now()
		requestInfoOf(c.parent).mcpTool = tool.Name
		result, out, err := handler(context.WithValue(ctx, mcpToolKey{}, tool.Name), req, in)
		waits, _ := any(in).(interface{ waitsForBuild() bool })
		timed := waits == nil || !waits.waitsForBuild()
		countToolCall(c.parent, tool.Name, toolOutcome(result, err), started, time.Since(started), timed)
		return result, out, err
	})
}

// toolOutcome names the outcome of a tool call: that of the API's answer
// when the API refused it, a client error when the client went away, and a
// server error for any other error
func toolOutcome(result *mcp.CallToolResult, err error) string {
	if err == nil && (result == nil || !result.IsError) {
		return OutcomeSuccess
	}
	var apiErr *mcpAPIError
	if errors.As(err, &apiErr) {
		return statusOutcome(apiErr.status)
	}
	if errors.Is(err, context.Canceled) {
		return OutcomeClientError
	}
	return OutcomeServerError
}

// newMCPServer makes the tools for one MCP request
func newMCPServer(c *mcpCaller) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "wakeci", Version: Version}, &mcp.ServerOptions{Instructions: mcpInstructions + mcpScopeNote(c.access)})
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	addTool(server, c, &mcp.Tool{Name: "whoami", Meta: mcp.Meta{"scope": "any"}, Annotations: readOnly,
		Description: "What this API key may do, and which server answers: its hostname, version and the range of build IDs it has."},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ mcpNoInput) (*mcp.CallToolResult, WhoAmIData, error) {
			var data WhoAmIData
			err := c.callJSON(ctx, "/api/whoami", &data)
			return nil, data, err
		})

	addTool(server, c, &mcp.Tool{Name: "list_jobs", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
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

	addTool(server, c, &mcp.Tool{Name: "find_builds", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
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

	addTool(server, c, &mcp.Tool{Name: "build_summary", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
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

	addTool(server, c, &mcp.Tool{Name: "analytics", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
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

	addTool(server, c, &mcp.Tool{Name: "job_insights", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
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

	addTool(server, c, &mcp.Tool{Name: "read_log", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
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

	addTool(server, c, &mcp.Tool{Name: "read_artifact", Meta: mcp.Meta{"scope": "read"}, Annotations: readOnly,
		Description: "A text artifact of a build, such as a JSON report, up to 256 KB."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcpReadArtifactInput) (*mcp.CallToolResult, any, error) {
			target := storageURL(in.Build, "artifacts/"+strings.TrimPrefix(in.Path, "/"))
			data, err := c.call(ctx, http.MethodGet, target, nil)
			if err != nil {
				return nil, nil, err
			}
			if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
				return nil, nil, &mcpAPIError{status: http.StatusUnsupportedMediaType, msg: fmt.Sprintf("%s isn't text; download it from %s", in.Path, target)}
			}
			text := string(data)
			if len(text) > mcpArtifactLimit {
				text = strings.ToValidUTF8(text[:mcpArtifactLimit], "") + "\n... (cut at 256 KB)"
			}
			return textResult(text), nil, nil
		})

	addTool(server, c, &mcp.Tool{Name: "run_job", Meta: mcp.Meta{"scope": "run"}, Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(false)},
		Description: "Start a build of a job. Needs the run scope. Parameters can't reference secrets or contain quotes or line breaks."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in mcpRunJobInput) (*mcp.CallToolResult, mcpStartedBuild, error) {
			form := url.Values{}
			for name, value := range in.Params {
				form.Set(name, value)
			}
			if in.Parent != 0 {
				form.Set(parentParam, strconv.Itoa(in.Parent))
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

	addTool(server, c, &mcp.Tool{Name: "abort_build", Meta: mcp.Meta{"scope": "run"}, Annotations: &mcp.ToolAnnotations{DestructiveHint: ptr(true)},
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
