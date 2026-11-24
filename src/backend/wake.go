package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// The wake command (docs/wake.md) is the wakeci binary run under the name
// wake. wakeci links it into wakeCommandDir and puts that first on every
// task's PATH, with WAKE_URL and WAKE_TOKEN, so a task starts builds and
// reads the API as its build. People run it with an API key in WAKE_TOKEN.

const wakeUsage = `wake starts wakeci builds and reads wakeci's API.

  wake start JOB... [NAME=VALUE...] [options]
      Start a build of each job; an "=" marks a parameter.
      --inherit          pass on this build's parameters that the job has;
                         NAME=VALUE changes one, NAME= leaves it at the job's default
      --each NAME        one build per line of standard input, the line as NAME
      --params-file F    parameters from the file F, one NAME=VALUE a line
      --wait             start the builds one at a time, each once the one before
                         has finished, and stop at the first that didn't finish
      --again ID         run build ID again, with its parameters
  wake api PATH
      Print the answer to GET PATH, e.g. wake api /api/build/$WAKE_BUILD_ID

In a task, WAKE_URL and WAKE_TOKEN are set, and the builds wake starts are
shown under the task's build. Elsewhere, set WAKE_URL to the server and
WAKE_TOKEN to an API key.

Everything is checked before the first build starts, so a mistyped job,
parameter or line starts nothing. A start is retried while the server
certainly took nothing (it restarts, or the caller sent too many requests),
never after.
`

// wakeCommandDir holds the wake command, a link to the running binary, and
// comes first on every task's PATH; empty if it couldn't be made
var wakeCommandDir string

// installWakeCommand links dir/wake to binary, replacing an older link
func installWakeCommand(dir, binary string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	link := filepath.Join(dir, "wake")
	next := link + ".new"
	if err := os.Remove(next); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Symlink(binary, next); err != nil {
		return err
	}
	return os.Rename(next, link)
}

// isWakeCommand reports whether the binary was run as the wake command
func isWakeCommand(arg0 string) bool {
	return filepath.Base(arg0) == "wake"
}

// wakeRetryDelays are the waits before each retry: about a minute in all,
// which covers a restart of wakeci
var wakeRetryDelays = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second}

const (
	// wakeRequestTimeout is how long wake waits for one answer
	wakeRequestTimeout = 30 * time.Second
	// wakeWaitSeconds is how long one request of --wait waits for a build
	wakeWaitSeconds = maxBuildWait
	// maxWakeAnswer is the most of an answer wake reads, except from api
	maxWakeAnswer = 64 << 20
)

var paramNameRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// runWake runs the wake command and returns its exit status: 0 if it did
// all it was asked, 1 if something failed, 2 if it was called wrongly
func runWake(args []string, getenv func(string) string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		say(stdout, "%s", wakeUsage)
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	client, err := newWakeClient(getenv, stdout, stderr)
	if err != nil {
		sayln(stderr, "wake:", err)
		return 2
	}
	switch args[0] {
	case "start":
		opts, err := parseStartArgs(args[1:])
		if err != nil {
			say(stderr, "wake start: %v\nRun wake help for how to use it.\n", err)
			return 2
		}
		return client.start(opts, getenv, stdin)
	case "api":
		if len(args) != 2 {
			sayln(stderr, "wake api: give one path, e.g. wake api /api/feed")
			return 2
		}
		body, err := client.request(http.MethodGet, args[1], nil, wakeRequestTimeout, -1)
		if err != nil {
			sayln(stderr, "wake api:", err)
			return 1
		}
		if _, err := stdout.Write(body); err != nil {
			return 1
		}
		return 0
	}
	say(stderr, "wake: no command %q; run wake help\n", args[0])
	return 2
}

type wakeClient struct {
	// base is WAKE_URL, ending in a slash
	base   string
	token  string
	http   *http.Client
	stdout io.Writer
	stderr io.Writer
	delays []time.Duration
	sleep  func(time.Duration)
}

func newWakeClient(getenv func(string) string, stdout, stderr io.Writer) (*wakeClient, error) {
	base := getenv("WAKE_URL")
	parsed, err := url.Parse(base)
	if base == "" || err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("set WAKE_URL to the wakeci server, e.g. https://ci.example.com/")
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	token := getenv("WAKE_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("set WAKE_TOKEN to an API key; a task gets its build's token")
	}
	return &wakeClient{
		base:   base,
		token:  token,
		http:   &http.Client{},
		stdout: stdout,
		stderr: stderr,
		delays: wakeRetryDelays,
		sleep:  time.Sleep,
	}, nil
}

// request sends a request to path, below WAKE_URL, and returns the answer's
// body. It retries while the server certainly took nothing: it couldn't be
// reached, it is stopping, or it limits the caller. A POST is never sent
// again once it may have reached the server, so a retry can't start a
// build twice. limit is the most of the answer it reads, -1 for all.
func (c *wakeClient) request(method, path string, form url.Values, timeout time.Duration, limit int64) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		body, retryAfter, retry, err := c.try(method, path, form, timeout, limit)
		if err == nil {
			return body, nil
		}
		if !retry || attempt >= len(c.delays) {
			return nil, err
		}
		delay := max(c.delays[attempt], retryAfter)
		say(c.stderr, "%v; trying again in %s\n", err, delay)
		c.sleep(delay)
	}
}

// try sends one request; retry says whether the server certainly took
// nothing, so it may be sent again
func (c *wakeClient) try(method, path string, form url.Values, timeout time.Duration, limit int64) (body []byte, retryAfter time.Duration, retry bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var wrote atomic.Bool
	ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			if info.Err == nil {
				wrote.Store(true)
			}
		},
	})
	var reader io.Reader
	if form != nil {
		reader = strings.NewReader(form.Encode())
	}
	// Always below WAKE_URL, so the token goes nowhere else
	req, err := http.NewRequestWithContext(ctx, method, c.base+strings.TrimLeft(path, "/"), reader)
	if err != nil {
		return nil, 0, false, err
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", "wake/"+Version)
	resp, err := c.http.Do(req)
	if err != nil {
		if method == http.MethodGet || !wrote.Load() {
			return nil, 0, true, fmt.Errorf("can't reach %s: %w", c.base, errors.Unwrap(err))
		}
		return nil, 0, false, fmt.Errorf("no answer from %s (%v); the build may have started, check the feed before starting it again", c.base, errors.Unwrap(err))
	}
	defer func() { _ = resp.Body.Close() }()
	var reading io.Reader = resp.Body
	if limit >= 0 {
		reading = io.LimitReader(resp.Body, limit)
	}
	body, err = io.ReadAll(reading)
	if err != nil {
		return nil, 0, method == http.MethodGet, fmt.Errorf("read the answer from %s: %w", c.base, err)
	}
	if resp.StatusCode < 300 {
		return body, 0, false, nil
	}
	text := strings.TrimSpace(string(body))
	err = fmt.Errorf("%d %s", resp.StatusCode, text)
	switch resp.StatusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		retry = true
		if seconds, convErr := strconv.Atoi(resp.Header.Get("Retry-After")); convErr == nil && seconds > 0 {
			retryAfter = time.Duration(seconds) * time.Second
		}
	case http.StatusBadRequest:
		// Nothing queued; the restarted server takes it
		retry = strings.Contains(text, ErrQueueStopping.Error())
	}
	return nil, retryAfter, retry, err
}

// getJSON reads the JSON answer to GET path into v
func (c *wakeClient) getJSON(path string, timeout time.Duration, v any) error {
	body, err := c.request(http.MethodGet, path, nil, timeout, maxWakeAnswer)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// wakeParam is a NAME=VALUE argument
type wakeParam struct {
	name, value string
}

type startOptions struct {
	jobs       []string
	params     []wakeParam
	inherit    bool
	each       string
	paramsFile string
	wait       bool
	again      int
}

// parseStartArgs reads the arguments of wake start. Options may come before,
// between and after jobs and parameters, and -- ends them.
func parseStartArgs(args []string) (*startOptions, error) {
	opts := &startOptions{}
	options := true
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if options && arg == "--" {
			options = false
			continue
		}
		if options && strings.HasPrefix(arg, "-") {
			name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
			takeValue := func() (string, error) {
				if hasValue {
					return value, nil
				}
				if i+1 >= len(args) {
					return "", fmt.Errorf("%s needs a value", arg)
				}
				i++
				return args[i], nil
			}
			var err error
			switch name {
			case "inherit":
				opts.inherit = true
			case "wait":
				opts.wait = true
			case "each":
				opts.each, err = takeValue()
				if err == nil && !paramNameRE.MatchString(opts.each) {
					err = fmt.Errorf("--each needs a parameter name, not %q", opts.each)
				}
			case "params-file":
				opts.paramsFile, err = takeValue()
			case "again":
				var text string
				text, err = takeValue()
				if err == nil {
					opts.again, err = strconv.Atoi(text)
					if err != nil || opts.again < 1 {
						err = fmt.Errorf("--again needs a build ID, not %q", text)
					}
				}
			default:
				err = fmt.Errorf("no option %s", arg)
			}
			if err != nil {
				return nil, err
			}
			if (name == "inherit" || name == "wait") && hasValue {
				return nil, fmt.Errorf("%s takes no value", "--"+name)
			}
			continue
		}
		if name, value, ok := strings.Cut(arg, "="); ok {
			if !paramNameRE.MatchString(name) {
				return nil, fmt.Errorf("%q isn't NAME=VALUE with a parameter name", arg)
			}
			opts.params = append(opts.params, wakeParam{name, value})
			continue
		}
		opts.jobs = append(opts.jobs, arg)
	}
	switch {
	case opts.again != 0 && (len(opts.jobs) > 0 || opts.inherit || opts.each != ""):
		return nil, fmt.Errorf("--again runs a build's job again: give no job, --inherit or --each with it")
	case opts.again == 0 && len(opts.jobs) == 0:
		return nil, fmt.Errorf("give a job to start")
	}
	return opts, nil
}

// wakeStart is one build that wake start is to start
type wakeStart struct {
	job    string
	params url.Values
	// line is the --each line it starts with, if any
	line string
}

func (s wakeStart) String() string {
	if s.line == "" {
		return s.job
	}
	return s.job + " (" + s.line + ")"
}

// start starts the builds opts asks for, after checking all of them
func (c *wakeClient) start(opts *startOptions, getenv func(string) string, stdin io.Reader) int {
	starts, problems, err := c.planStarts(opts, getenv, stdin)
	if err != nil {
		sayln(c.stderr, "wake start:", err)
		return 1
	}
	if len(problems) > 0 {
		for _, problem := range problems {
			sayln(c.stderr, problem)
		}
		sayln(c.stderr, "Nothing was started.")
		return 1
	}
	if len(starts) == 0 {
		sayln(c.stderr, "No lines on standard input, so nothing was started.")
		return 0
	}
	for i, s := range starts {
		id, err := c.run(s)
		if err != nil {
			say(c.stderr, "Could not start %s: %v\n", s, err)
			c.notStarted(starts[i+1:])
			return 1
		}
		say(c.stdout, "Started %s #%d: %sbuild/%d\n", s.job, id, c.base, id)
		if !opts.wait {
			continue
		}
		status, err := c.waitFor(id)
		if err != nil {
			say(c.stderr, "Could not follow %s #%d: %v\n", s.job, id, err)
			c.notStarted(starts[i+1:])
			return 1
		}
		say(c.stdout, "%s #%d: %s\n", s.job, id, status)
		if status != StatusFinished {
			c.notStarted(starts[i+1:])
			return 1
		}
	}
	return 0
}

func (c *wakeClient) notStarted(rest []wakeStart) {
	for _, s := range rest {
		say(c.stderr, "Not started: %s\n", s)
	}
}

// planStarts returns the builds to start, or the problems that keep wake
// from starting any. err is for what keeps it from checking.
func (c *wakeClient) planStarts(opts *startOptions, getenv func(string) string, stdin io.Reader) ([]wakeStart, []string, error) {
	var jobs []*JobsListData
	if err := c.getJSON("api/jobs/", wakeRequestTimeout, &jobs); err != nil {
		return nil, nil, fmt.Errorf("read the jobs: %w", err)
	}
	byName := make(map[string]*JobsListData, len(jobs))
	for _, job := range jobs {
		// Not in wakeci's answer; a proxy's would otherwise panic wake
		if job != nil {
			byName[job.Name] = job
		}
	}
	var who WhoAmIData
	if err := c.getJSON("api/whoami", wakeRequestTimeout, &who); err != nil {
		return nil, nil, fmt.Errorf("read what this token may do: %w", err)
	}
	// The server checks it again; this only saves a fan-out stopped halfway
	trusted := who.Admin
	for _, grant := range who.Grants {
		trusted = trusted || grant.HasScope(ScopeWrite)
	}

	var problems []string
	var fileParams []wakeParam
	if opts.paramsFile != "" {
		var err error
		fileParams, err = readParamsFile(opts.paramsFile)
		if err != nil {
			problems = append(problems, err.Error())
		}
	}
	names := opts.jobs
	var again url.Values
	if opts.again != 0 {
		var build GetBuildPayload
		if err := c.getJSON("api/build/"+strconv.Itoa(opts.again), wakeRequestTimeout, &build); err != nil {
			return nil, nil, fmt.Errorf("read build %d: %w", opts.again, err)
		}
		if build.StatusUpdate == nil {
			return nil, nil, fmt.Errorf("read build %d: the answer has no status_update", opts.again)
		}
		names = []string{build.StatusUpdate.Name}
		again = url.Values{}
		for _, param := range build.StatusUpdate.Params {
			for name, value := range param {
				again.Set(name, value)
			}
		}
	}
	var inherited url.Values
	if opts.inherit {
		var err error
		inherited, err = url.ParseQuery(getenv("WAKE_JOB_PARAMS"))
		if err != nil {
			problems = append(problems, "WAKE_JOB_PARAMS can't be read: "+err.Error())
		}
	}
	var lines []string
	if opts.each != "" {
		scanner := bufio.NewScanner(stdin)
		scanner.Buffer(make([]byte, 64<<10), 1<<20)
		for scanner.Scan() {
			if line := strings.TrimSpace(scanner.Text()); line != "" {
				lines = append(lines, line)
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, nil, fmt.Errorf("read standard input: %w", err)
		}
	}

	var starts []wakeStart
	for _, name := range names {
		job, ok := byName[name]
		if !ok {
			problems = append(problems, "There is no job "+name+".")
			continue
		}
		if job.Active != "true" {
			problems = append(problems, name+" is disabled.")
		}
		has := map[string]bool{}
		for _, param := range job.DefaultParams {
			for paramName := range param {
				has[paramName] = true
			}
		}
		params := url.Values{}
		for paramName, values := range inherited {
			if has[paramName] && len(values) > 0 {
				params.Set(paramName, values[0])
			}
		}
		for paramName, values := range again {
			if has[paramName] {
				params.Set(paramName, values[0])
			}
		}
		for _, sets := range [][]wakeParam{fileParams, opts.params} {
			for _, param := range sets {
				if !has[param.name] {
					problems = append(problems, name+" has no parameter "+param.name+".")
					continue
				}
				params.Set(param.name, param.value)
			}
		}
		if opts.again != 0 {
			params.Set(againParam, strconv.Itoa(opts.again))
		}
		if opts.each == "" {
			starts = append(starts, wakeStart{job: name, params: params})
			continue
		}
		if !has[opts.each] {
			problems = append(problems, name+" has no parameter "+opts.each+" (--each).")
			continue
		}
		for _, line := range lines {
			each := url.Values{}
			for k, v := range params {
				each[k] = v
			}
			each.Set(opts.each, line)
			starts = append(starts, wakeStart{job: name, params: each, line: opts.each + "=" + line})
		}
	}
	if !trusted {
		for _, s := range starts {
			for paramName, values := range s.params {
				if problem := untrustedParamProblem(values[0]); problem != "" {
					problems = append(problems, fmt.Sprintf("%s: parameter %s %s, which only those who may edit jobs may send; %s= leaves it at the job's default.", s, paramName, problem, paramName))
				}
			}
		}
	}
	return starts, problems, nil
}

// readParamsFile reads NAME=VALUE lines; blank lines and lines starting
// with # are left out
func readParamsFile(path string) ([]wakeParam, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("--params-file: %w", err)
	}
	var params []wakeParam
	for n, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		if !ok || !paramNameRE.MatchString(name) {
			return nil, fmt.Errorf("%s:%d isn't NAME=VALUE", path, n+1)
		}
		params = append(params, wakeParam{name, value})
	}
	return params, nil
}

// run starts one build and returns its ID
func (c *wakeClient) run(s wakeStart) (int, error) {
	body, err := c.request(http.MethodPost, "api/job/"+url.PathEscape(s.job)+"/run", s.params, wakeRequestTimeout, maxWakeAnswer)
	if err != nil {
		return 0, err
	}
	id, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil {
		return 0, fmt.Errorf("unexpected answer %q", body)
	}
	return id, nil
}

// waitFor waits until build id has ended and returns its status
func (c *wakeClient) waitFor(id int) (ItemStatus, error) {
	path := fmt.Sprintf("api/build/%d?wait=%d", id, wakeWaitSeconds)
	for {
		var build GetBuildPayload
		if err := c.getJSON(path, time.Duration(wakeWaitSeconds)*time.Second+wakeRequestTimeout, &build); err != nil {
			return "", err
		}
		// wakeci always sends it; an answer without it, say a proxy's page,
		// would otherwise be asked for again at once, forever
		if build.StatusUpdate == nil {
			return "", fmt.Errorf("build %d: the answer has no status_update", id)
		}
		if build.Ended {
			return build.StatusUpdate.Status, nil
		}
	}
}

// say writes wake's output; there is nowhere to report a failed write
func say(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func sayln(w io.Writer, args ...any) {
	_, _ = fmt.Fprintln(w, args...)
}
