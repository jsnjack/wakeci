# Analytics: build stats, the analytics page and job insights

Read this before changing what wakeci records about builds, the analytics
page (`/analytics`), a job's Insights tab (`/job/{name}/insights`), or their
API routes and MCP tools.

## Two sources

- **Build stats** (`stats.go`): one small record per build in the
  `buildstats` bucket, kept for 400 days (`statsRetention`), long after
  cleanup has deleted the build's history entry. The analytics page reads
  only these.
- **Build history**: the `history` bucket, which keeps the last
  `buildHistorySize` builds of all jobs (200 by default) with every task's
  status and duration. Job insights read it, so they reach back only as far
  as the history does; the page says how many of the job's builds that is.

Records are per build, not daily totals: percentiles stay exact, a request
can filter by job, trigger and access when it reads them, and writing the
same record twice changes nothing, which keeps the write path and the
backfill simple. A record is about 250 bytes.

## What a record holds

`buildStats`: ID, job, status, `queuedAt` (when `CreateBuild` made it),
`startedAt`, duration, `ended`, `trigger` and `failedTask` (the main task that
stopped it).

- **Written with the history entry.** `Build.records` builds the history
  entry, the stats record and the queue record from one locked read, and
  `BroadcastUpdate` saves them in one transaction, so they never disagree. A
  new measure goes there; older records won't have it.
- **`ended`** is set by `setDuration`, once the on_* and finally tasks ran.
  The status turns final before that, with a duration of 0, so durations
  only count from ended records.
- **`trigger`** is who started the build: `interval` (cron, `Job.Run`),
  `password` (the password, in a session or Basic auth), `sso` with the
  email, or `apikey` with the key's ID and its creator. `HandleRunJob` reads
  it from the request's `requestInfo` (`triggerOf`), which the auth
  middleware filled, so MCP's `run_job` counts as its key. Builds from before
  wakeci recorded it have none, shown as `unknown`.
- **Restarts.** A queued build keeps `queuedAt` and `trigger` in its queue
  record (`queuedBuild`). A build that ran when wakeci stopped is aborted in
  its record too (`markStatsAborted`, from `markStaleBuildsAborted`), keeping
  its trigger.
- **Backfill.** At startup, after `MarkInterruptedBuilds`,
  `BackfillBuildStats` makes a record for every build in the history that has
  none, with the failed task's name from the build plan. It runs on every
  start, so builds that ended under an older version after a downgrade are
  counted too. Builds that never started have no day to count on and are
  skipped.
- **Pruning.** The cleaner (`Cleaner.Clean`, every 15 minutes) deletes
  records older than `statsRetention`. Records are in the order builds were
  created, so it stops at the first newer one.

## The analytics page

`GET /api/analytics/?days=&job=&trigger=` (`HandleAnalytics`,
`computeAnalytics`) answers everything the page shows for 1 to 365 days, the
last one today:

- **Days are the server's** (`time.Local`), like the API keys' usage. A build
  counts on the day it was queued, or started for backfilled builds.
- **Success rate** is finished builds among finished, failed and timed out
  ones. Aborted builds are left out: a person or a restart stopped them.
- **Each day's rate, duration and wait cover the 7 days up to it**
  (`rollingDays`), so they don't jump at weekends. Duration counts finished
  builds that ended; wait is `startedAt − queuedAt`.
- **The previous period** (as many days before the range) is compared only
  if the stats reach back to its start.
- **`failing`**, in each job of the answer, is set when the job's last
  build that wasn't aborted failed or timed out; aborted builds don't end
  the streak. The page doesn't show it: with deleted and test jobs, whose
  stats outlive them, the list grew long; the jobs table shows each job's
  last builds instead. MCP's `analytics` tool reports it.
- **Reading**: in a read transaction, from the newest record back until one
  is older than the range by more than `statsOrderSlack` (2 days), since a
  build may wait in the queue and backfilled records are ordered by start.

The page keeps the range, job and trigger in the address. The duration chart
shows one job: the one filtered to, the one picked in the chart, or the one
with the most build time, since jobs of seconds and of hours make a median
of all of them say little.

## Job insights

`GET /api/job/{name}/insights?builds=` (`HandleJobInsights`,
`computeInsights`) reads the job's last builds from the history, 40 unless
asked for 1 to 500, leaving out builds still in the queue. Task names come
from each build's plan (`getBuildConfig`), read after the transaction.

- **Tasks** are the main tasks by name, in the order of the last build: runs,
  failures, skips, duration of finished runs, share of the builds' time, and
  the change of the median of the last 10 finished runs against the runs
  before (needs 15).
- **Flaky**: a failed or timed out build whose next build with the same
  parameters, aborted ones left out, finished.
- **Time to fix**: from the start of the first build of a failing streak to
  the end of the finished build after it.
- **Parameters** with more than one value, each value with its success rate.

## Who sees what

- Totals are computed only from the jobs the caller may read
  (`Access.Reads` while reading), not filtered afterwards, so someone limited
  to some jobs can't work out the others from the totals.
- Everyone sees trigger kinds. Who started builds (`people`: emails and API
  key names) is filled only for the admin password (`Access.Full`), because
  the feed doesn't show who started a build either.
- Both routes need the `read` scope and are in both routers; insights also
  check the job (`jobFromName`). `/api/analytics/` is open to anyone with
  `read` on some job, like the feed, so it is listed as open in
  `TestEveryAPIRouteChecksAccess`. They change nothing, so they aren't
  audited. MCP has them as `analytics` and `job_insights`.

## Frontend

`AnalyticsView.vue` and `JobInsightsView.vue`, with `analytics.js` for
formatting. The charts are small SVG components in `components/charts/`, not
a chart library, which would be the largest chunk under Vite's 500 kB limit.
They take their width from their element (`chartWidth.js`), so text keeps
its size on a phone. Colours are in `assets/charts.css`; failures are also
hatched, outcome strips also use height (short finished, full failed), and
every chart has a legend and a tooltip per mark (`<title>`), so colour
never carries meaning alone.

Both pages subscribe to `build:update:` and fetch again, at most every few
seconds, when a build ends (final status and a duration), and when the
WebSocket reconnects.
