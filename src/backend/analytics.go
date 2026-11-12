package main

import (
	"cmp"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	bolt "go.etcd.io/bbolt"
)

// The analytics page reads the build stats (stats.go). Days are those of the
// server's time zone, like the API keys' usage. A build counts on the day it
// was queued; the success rate counts finished builds against failed and
// timed out ones, leaving out aborted builds, which a person or a restart
// stopped.

const (
	// defaultAnalyticsDays is the range when a request names none
	defaultAnalyticsDays = 30
	// maxAnalyticsDays is the longest range, within statsRetention
	maxAnalyticsDays = 365
	// rollingDays is how many days each day's rates and durations cover
	rollingDays = 7
	// recentOutcomes is how many of a job's last builds it lists
	recentOutcomes = 20
	// maxAnalyticsPeople is how many people and keys the admin sees
	maxAnalyticsPeople = 20
	// statsOrderSlack is how much older than the range a record may be and
	// still be followed by records in it: records are in the order builds
	// were created, and backfilled ones by when they started
	statsOrderSlack = 2 * 24 * time.Hour
)

// Percentiles are the median and the 95th percentile of durations, in
// nanoseconds, interpolated between values; zero without values
type Percentiles struct {
	// Count is how many durations they come from
	Count int           `json:"count"`
	P50   time.Duration `json:"p50" swaggertype:"integer"`
	P95   time.Duration `json:"p95" swaggertype:"integer"`
}

// StatusCounts are builds by their last status
type StatusCounts struct {
	Builds   int `json:"builds"`
	Finished int `json:"finished"`
	Failed   int `json:"failed"`
	TimedOut int `json:"timedOut"`
	Aborted  int `json:"aborted"`
	// SuccessRate is finished builds among finished, failed and timed out
	// ones, from 0 to 1; null without such builds
	SuccessRate *float64 `json:"successRate" extensions:"x-nullable"`
}

func (c *StatusCounts) add(status ItemStatus) {
	c.Builds++
	switch status {
	case StatusFinished:
		c.Finished++
	case StatusFailed:
		c.Failed++
	case StatusTimedOut:
		c.TimedOut++
	case StatusAborted:
		c.Aborted++
	}
}

func (c *StatusCounts) rate() {
	c.SuccessRate = successRate(c.Finished, c.Failed+c.TimedOut)
}

func successRate(finished, failed int) *float64 {
	if finished+failed == 0 {
		return nil
	}
	rate := float64(finished) / float64(finished+failed)
	return &rate
}

// AnalyticsData is what the analytics page shows for a range of days
type AnalyticsData struct {
	// Days is how many days the range has, the last one today
	Days int `json:"days"`
	// From is the range's first day and Today its last, such as 2026-11-12,
	// in the server's time zone
	From  string `json:"from"`
	Today string `json:"today"`
	// Since is when the oldest build wakeci has stats of was queued or
	// started; null if it has none. Days before it have no data
	Since *time.Time `json:"since" extensions:"x-nullable"`
	// Totals are the builds in the range that have their last status, with
	// the duration of those that finished and the queue wait of those that
	// started
	Totals AnalyticsTotals `json:"totals"`
	// PreviousSuccessRate is the success rate of as many days before From;
	// null if wakeci has no stats of all of them or they had no builds
	PreviousSuccessRate *float64 `json:"previousSuccessRate" extensions:"x-nullable"`
	// ByDay has every day of the range, oldest first
	ByDay []AnalyticsDay `json:"byDay"`
	// Starts counts the builds that started in the range by weekday, Monday
	// first, and hour
	Starts [][]int `json:"starts"`
	// Triggers counts the builds by who or what started them
	Triggers []TriggerCount `json:"triggers"`
	// People are the single sign-on users and API keys that started the
	// most builds, for the admin password only (showsPeople)
	People      []PersonCount `json:"people"`
	ShowsPeople bool          `json:"showsPeople"`
	// Jobs are the jobs with builds in the range
	Jobs []JobAnalytics `json:"jobs"`
}

// AnalyticsTotals are a range's builds
type AnalyticsTotals struct {
	StatusCounts
	// Duration is that of the builds that finished
	Duration Percentiles `json:"duration"`
	// Wait is from queued to started, of builds queued since wakeci
	// recorded it
	Wait Percentiles `json:"wait"`
}

// AnalyticsDay is a day's builds. The success rate, duration and wait cover
// the rollingDays days up to and including it, so they don't jump at
// weekends
type AnalyticsDay struct {
	Day      string      `json:"day"`
	Finished int         `json:"finished"`
	Failed   int         `json:"failed"`
	TimedOut int         `json:"timedOut"`
	Aborted  int         `json:"aborted"`
	Rate     *float64    `json:"successRate" extensions:"x-nullable"`
	Duration Percentiles `json:"duration"`
	Wait     Percentiles `json:"wait"`
}

// TriggerCount is how many builds a kind of trigger started
type TriggerCount struct {
	// Kind is interval, password, sso, apikey or unknown (builds from before
	// wakeci recorded it)
	Kind   string `json:"kind"`
	Builds int    `json:"builds"`
}

// PersonCount is how many builds a single sign-on user or an API key started
type PersonCount struct {
	// Kind is sso or apikey
	Kind string `json:"kind"`
	// User is the single sign-on email, or who created the API key (empty
	// for the admin password)
	User string `json:"user"`
	// APIKey is the key's ID and KeyName its name, empty once revoked
	APIKey  string `json:"apiKey,omitempty"`
	KeyName string `json:"keyName,omitempty"`
	Builds  int    `json:"builds"`
}

// JobAnalytics is a job's builds in the range
type JobAnalytics struct {
	Name string `json:"name"`
	StatusCounts
	// Duration is that of the builds that finished
	Duration Percentiles `json:"duration"`
	// BuildTime adds up how long its builds ran, in nanoseconds
	BuildTime time.Duration `json:"buildTime" swaggertype:"integer"`
	Wait      Percentiles   `json:"wait"`
	// Recent are its last builds, at most 20, oldest first
	Recent []BuildOutcome `json:"recent"`
	// Failing is set if its last build that wasn't aborted failed or timed
	// out
	Failing *FailingStreak `json:"failing" extensions:"x-nullable"`
}

// BuildOutcome is a build's result
type BuildOutcome struct {
	ID        int        `json:"id"`
	Status    ItemStatus `json:"status"`
	StartedAt time.Time  `json:"startedAt"`
	// Duration is in nanoseconds, 0 until the build ended
	Duration time.Duration `json:"duration" swaggertype:"integer"`
	// FailedTask is the main task that stopped the build, if any
	FailedTask string `json:"failedTask,omitempty"`
}

// FailingStreak is the builds of a job that failed or timed out in a row,
// up to its last one; aborted builds don't end it
type FailingStreak struct {
	Builds int `json:"builds"`
	// First is the streak's first build and Last its last
	First BuildOutcome `json:"first"`
	Last  BuildOutcome `json:"last"`
}

// analyticsQuery is what a request asks for
type analyticsQuery struct {
	days int
	// job and trigger, if set, keep only builds of that job and trigger kind
	job     string
	trigger string
}

func analyticsQueryFrom(values url.Values) (analyticsQuery, error) {
	q := analyticsQuery{days: defaultAnalyticsDays, job: values.Get("job"), trigger: values.Get("trigger")}
	if s := values.Get("days"); s != "" {
		days, err := strconv.Atoi(s)
		if err != nil || days < 1 || days > maxAnalyticsDays {
			return q, fmt.Errorf("days must be a number from 1 to %d", maxAnalyticsDays)
		}
		q.days = days
	}
	if q.trigger != "" && !slices.Contains(triggerKinds, q.trigger) {
		return q, fmt.Errorf("trigger must be one of %v", triggerKinds)
	}
	return q, nil
}

func (q *analyticsQuery) matches(s *buildStats) bool {
	return (q.job == "" || s.Job == q.job) && (q.trigger == "" || s.triggerKind() == q.trigger)
}

// analyticsRange is the instants a query covers
type analyticsRange struct {
	// from is the first day's start, end the end of today
	from, end time.Time
	// rolling is where the first day's rolling window starts, previous
	// where the days before from start
	rolling, previous time.Time
}

func rangeOf(q analyticsQuery, now time.Time) analyticsRange {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	from := today.AddDate(0, 0, -(q.days - 1))
	return analyticsRange{
		from:     from,
		end:      today.AddDate(0, 0, 1),
		rolling:  from.AddDate(0, 0, -(rollingDays - 1)),
		previous: from.AddDate(0, 0, -q.days),
	}
}

// earliest is the oldest instant the query needs records of
func (r analyticsRange) earliest() time.Time {
	if r.rolling.Before(r.previous) {
		return r.rolling
	}
	return r.previous
}

// percentiles returns the median and 95th percentile of values, which it
// sorts
func percentiles(values []time.Duration) Percentiles {
	if len(values) == 0 {
		return Percentiles{}
	}
	slices.Sort(values)
	at := func(q float64) time.Duration {
		i := q * float64(len(values)-1)
		lo, hi := int(math.Floor(i)), int(math.Ceil(i))
		return values[lo] + time.Duration(math.Round(float64(values[hi]-values[lo])*(i-float64(lo))))
	}
	return Percentiles{Count: len(values), P50: at(0.5), P95: at(0.95)}
}

// durationOf returns how long a build ran, if it finished and ended
func durationOf(s *buildStats) (time.Duration, bool) {
	return s.Duration, s.Status == StatusFinished && s.Ended && s.Duration > 0
}

func outcomeOf(s *buildStats) BuildOutcome {
	return BuildOutcome{ID: s.ID, Status: s.Status, StartedAt: s.StartedAt, Duration: s.Duration, FailedTask: s.FailedTask}
}

// failingStreak returns the failed or timed out builds up to the last one,
// in order, or nil if the last build that wasn't aborted finished
func failingStreak(builds []*buildStats) *FailingStreak {
	var streak *FailingStreak
	for i := len(builds) - 1; i >= 0; i-- {
		b := builds[i]
		switch b.Status {
		case StatusAborted:
			continue
		case StatusFailed, StatusTimedOut:
			if streak == nil {
				streak = &FailingStreak{Last: outcomeOf(b)}
			}
			streak.Builds++
			streak.First = outcomeOf(b)
			continue
		}
		break
	}
	return streak
}

// jobAccumulator collects a job's builds in the range
type jobAccumulator struct {
	JobAnalytics
	durations, waits []time.Duration
	builds           []*buildStats
}

// computeAnalytics returns the analytics of records, which the caller has
// limited to what the request may read. now sets today and the time zone;
// since is when the oldest record of all is from, zero if there are none.
// People are listed only if withPeople
func computeAnalytics(records []*buildStats, q analyticsQuery, now, since time.Time, withPeople bool) *AnalyticsData {
	loc := now.Location()
	r := rangeOf(q, now)
	data := &AnalyticsData{
		Days:        q.days,
		From:        r.from.Format(time.DateOnly),
		Today:       r.from.AddDate(0, 0, q.days-1).Format(time.DateOnly),
		ByDay:       make([]AnalyticsDay, q.days),
		Starts:      make([][]int, 7),
		Triggers:    []TriggerCount{},
		People:      []PersonCount{},
		ShowsPeople: withPeople,
		Jobs:        []JobAnalytics{},
	}
	if !since.IsZero() {
		data.Since = &since
	}
	for i := range data.Starts {
		data.Starts[i] = make([]int, 24)
	}

	// Each day's builds, from the start of the first day's window
	windowDays := q.days + rollingDays - 1
	perDay := make([][]*buildStats, windowDays)
	var inRange []*buildStats
	var previous StatusCounts
	for _, s := range records {
		if !s.final() || !q.matches(s) {
			continue
		}
		at := s.at().In(loc)
		if at.IsZero() || !at.Before(r.end) {
			continue
		}
		if !at.Before(r.rolling) {
			perDay[dayIndex(r.rolling, at)] = append(perDay[dayIndex(r.rolling, at)], s)
		}
		if !at.Before(r.from) {
			inRange = append(inRange, s)
		} else if !at.Before(r.previous) {
			previous.add(s.Status)
		}
	}
	// In the order builds were queued
	slices.SortFunc(inRange, func(a, b *buildStats) int {
		return cmp.Or(a.at().Compare(b.at()), cmp.Compare(a.ID, b.ID))
	})

	if !since.IsZero() && !since.After(r.previous) {
		previous.rate()
		data.PreviousSuccessRate = previous.SuccessRate
	}

	for i := range q.days {
		day := &data.ByDay[i]
		day.Day = r.from.AddDate(0, 0, i).Format(time.DateOnly)
		var window StatusCounts
		var durations, waits []time.Duration
		for w := i; w < i+rollingDays; w++ {
			for _, s := range perDay[w] {
				window.add(s.Status)
				if d, ok := durationOf(s); ok {
					durations = append(durations, d)
				}
				if wait, ok := s.wait(); ok {
					waits = append(waits, wait)
				}
				if w == i+rollingDays-1 {
					switch s.Status {
					case StatusFinished:
						day.Finished++
					case StatusFailed:
						day.Failed++
					case StatusTimedOut:
						day.TimedOut++
					case StatusAborted:
						day.Aborted++
					}
				}
			}
		}
		window.rate()
		day.Rate = window.SuccessRate
		day.Duration = percentiles(durations)
		day.Wait = percentiles(waits)
	}

	var durations, waits []time.Duration
	triggers := map[string]int{}
	type personKey struct{ kind, user, apiKey string }
	people := map[personKey]int{}
	jobs := map[string]*jobAccumulator{}
	for _, s := range inRange {
		data.Totals.add(s.Status)
		job := jobs[s.Job]
		if job == nil {
			job = &jobAccumulator{JobAnalytics: JobAnalytics{Name: s.Job}}
			jobs[s.Job] = job
		}
		job.add(s.Status)
		job.builds = append(job.builds, s)
		if d, ok := durationOf(s); ok {
			durations = append(durations, d)
			job.durations = append(job.durations, d)
		}
		if s.Ended {
			job.BuildTime += s.Duration
		}
		if wait, ok := s.wait(); ok {
			waits = append(waits, wait)
			job.waits = append(job.waits, wait)
		}
		if !s.StartedAt.IsZero() {
			started := s.StartedAt.In(loc)
			// Monday first
			data.Starts[(int(started.Weekday())+6)%7][started.Hour()]++
		}
		triggers[s.triggerKind()]++
		if withPeople && (s.Trigger.User != "" || s.Trigger.APIKey != "") {
			people[personKey{s.Trigger.Kind, s.Trigger.User, s.Trigger.APIKey}]++
		}
	}
	data.Totals.rate()
	data.Totals.Duration = percentiles(durations)
	data.Totals.Wait = percentiles(waits)

	for _, kind := range triggerKinds {
		if n := triggers[kind]; n > 0 {
			data.Triggers = append(data.Triggers, TriggerCount{Kind: kind, Builds: n})
		}
	}
	slices.SortStableFunc(data.Triggers, func(a, b TriggerCount) int { return cmp.Compare(b.Builds, a.Builds) })

	for key, n := range people {
		data.People = append(data.People, PersonCount{Kind: key.kind, User: key.user, APIKey: key.apiKey, Builds: n})
	}
	slices.SortFunc(data.People, func(a, b PersonCount) int {
		return cmp.Or(cmp.Compare(b.Builds, a.Builds), cmp.Compare(a.User, b.User), cmp.Compare(a.APIKey, b.APIKey))
	})
	if len(data.People) > maxAnalyticsPeople {
		data.People = data.People[:maxAnalyticsPeople]
	}

	for _, job := range jobs {
		job.rate()
		job.Duration = percentiles(job.durations)
		job.Wait = percentiles(job.waits)
		recent := job.builds[max(0, len(job.builds)-recentOutcomes):]
		job.Recent = make([]BuildOutcome, 0, len(recent))
		for _, s := range recent {
			job.Recent = append(job.Recent, outcomeOf(s))
		}
		job.Failing = failingStreak(job.builds)
		data.Jobs = append(data.Jobs, job.JobAnalytics)
	}
	// The least reliable first, as the page shows them
	slices.SortFunc(data.Jobs, func(a, b JobAnalytics) int {
		return cmp.Or(cmp.Compare(rateOrOne(a.SuccessRate), rateOrOne(b.SuccessRate)), cmp.Compare(a.Name, b.Name))
	})
	return data
}

func rateOrOne(rate *float64) float64 {
	if rate == nil {
		return 1
	}
	return *rate
}

// dayIndex returns how many days at is after the day that starts at start,
// counting calendar days so a change to or from summer time doesn't shift
// it
func dayIndex(start, at time.Time) int {
	day := time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, start.Location())
	return int(math.Round(day.Sub(start).Hours() / 24))
}

// readBuildStats returns the records of builds queued or started at or after
// from that access may read, and when the oldest record of all is from
func readBuildStats(tx *bolt.Tx, from time.Time, access *Access) ([]*buildStats, time.Time) {
	var records []*buildStats
	var since time.Time
	b := tx.Bucket(BuildStatsBucket)
	if b == nil {
		return records, since
	}
	c := b.Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		var s buildStats
		if json.Unmarshal(v, &s) == nil && !s.at().IsZero() {
			since = s.at()
			break
		}
	}
	cutoff := from.Add(-statsOrderSlack)
	for k, v := c.Last(); k != nil; k, v = c.Prev() {
		var s buildStats
		if err := json.Unmarshal(v, &s); err != nil {
			L.Error("decode build stats", "build", binary.BigEndian.Uint64(k), "err", err)
			continue
		}
		if at := s.at(); !at.IsZero() && at.Before(cutoff) {
			break
		}
		if access.Reads(s.Job) {
			records = append(records, &s)
		}
	}
	return records, since
}

// addKeyNames names the API keys in people that still exist
func addKeyNames(tx *bolt.Tx, people []PersonCount) {
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

// HandleAnalytics returns what the analytics page shows
// @Summary      Analytics of builds over a range of days
// @Description  Builds by day and status, success rate, duration and queue wait, when builds start, who starts them, and each job's numbers, with the jobs that are failing now. Only the jobs the caller may read count. Days are in the server's time zone; the success rate, duration and wait of each day cover the 7 days up to it. people (who started builds, by person and API key) is filled for the admin password only. API key scope: read
// @Tags         analytics
// @Produce      json
// @Param        days     query   integer   false  "How many days, the last one today, 1 to 365; 30 if not set"
// @Param        job      query   string    false  "Only builds of this job"
// @Param        trigger  query   string    false  "Only builds started by: interval, password, sso, apikey or unknown (before wakeci recorded it)"
// @Success      200      {object}   AnalyticsData
// @Success      304      {string}   string
// @Failure      400      {string}   string
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      429      {string}   string
// @Failure      500      {string}   string
// @Router       /api/analytics/ [get]
func HandleAnalytics(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	q, err := analyticsQueryFrom(r.URL.Query())
	if err != nil {
		logger.Warn("invalid analytics request", "err", err)
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	access := accessFromContext(r.Context())
	withPeople := access != nil && access.Full
	now := time.Now()
	var data *AnalyticsData
	err = DB.View(func(tx *bolt.Tx) error {
		records, since := readBuildStats(tx, rangeOf(q, now).earliest(), access)
		data = computeAnalytics(records, q, now, since, withPeople)
		addKeyNames(tx, data.People)
		return nil
	})
	if err != nil {
		logger.Error("read build stats", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	writeCachedJSON(w, r, logger, data)
}
