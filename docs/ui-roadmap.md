# UI roadmap: feed, live updates, build page, task logs

Read this before changing the feed, the build page, task logs, the jobs page
or the run form. It lists the changes planned for them, the evidence behind
each, and the order. When an item ships, delete it here and name it (e.g.
"F1") in the commit message.

## Evidence

- **Real use.** Two days of a busy installation in November 2026: 1,815
  builds, up to 146 an hour (2.4 a minute). A job that starts eight test
  jobs for each pushed commit, and those eight jobs, made 68% of the builds;
  a commit's builds (median 10) were spread over 28 feed rows, between those
  of other commits. 19% were scheduled or maintenance builds, 98% of which
  passed.
- **What the feed showed.** The first three non-empty parameters: a commit
  hash cut to 27 characters, the branch, and a parameter that is the same on
  every build. The author was the fourth, so hidden on every row.
- **Failed builds.** The failed task was row 14 of about 20 (median), below
  the fold. Failing task logs were 1,300 to 24,000 lines (110 KB to 2.4 MB),
  with the failed tests and the totals in the last 20 lines.
- **Screens.** On 1440×900, 7 feed rows fit (96 px each). On a 390 px phone
  the header hides every link except Log out.
- **Other tools.** Sources at the end.

## Feed

- **F1. Show what a build is about.** The chips already leave out empty
  parameters and those at the job's current default, with the name in a
  tooltip. Left: 40-character hex values shortened to 7, copying the full
  value. Effort S.
- **F2. Group builds under the build that started them.** `POST
  /api/job/{name}/run` accepts `parent=<build id>`; a job that starts others
  passes `parent=${WAKE_BUILD_ID}`. The feed shows one row per parent with a
  strip of its children's statuses and the worst of them as the row's
  status; expanding lists the children. The build page shows the strip too,
  to move between siblings. Explicit rather than guessed: grouping by an
  equal commit parameter would merge re-runs and miss jobs without it. Needs
  the parent in the build's history and stats records, the API docs and an
  audit attribute. Effort L.
- **F3. One line per build.** About 40 px: status, number, job, chips,
  progress or duration, time. The row is the link; Abort and Start now on
  hover and in a "⋯" menu, only when they can act (today they show disabled).
  Queued builds say how long they have waited and their place in the queue.
  Effort M.
- **F4. Quick filters.** Chips that write the existing `q`: Failed, Running
  and queued, Hide scheduled, several jobs, and Mine. Where builds are started
  by another job or a shared credential, people are identified by a
  parameter, so Mine is a filter each person sets once (e.g.
  `AUTHOR:alice`), kept in the browser. "Started by me" from the recorded
  trigger can come later; the feed doesn't send the trigger today. Effort M.
- **F5. Fold routine scheduled builds.** Passing builds of a scheduled job
  fold into one row at the place of the newest ("health_check ✓ ×12 since
  09:00"); a failure always gets its own row. Effort S.
- **F6. Optional job icon and title.** `icon:` (an emoji) shown before the
  name, and `title:` as a template over the params, like GitHub's
  `run-name`. Not a colour per job: colour means status. Lower value than
  F1, which covers most of the need. Effort S.

## Live updates

- **L1. Keep the list still while the user reads.** Update rows in place
  with a short highlight on a status change. While the user has scrolled down
  or hovers a row, collect new builds behind a "3 new builds" pill; insert
  them on click or at the top. Today `FeedView.applyUpdate` adds and
  re-sorts at once. Effort S.
- **L2. Load, then stream.** A running task opened mid-way shows only what
  arrives after the page subscribed: `wsclient.go` sends no earlier lines and
  `TaskItem` starts empty. Fetch what is written, then append streamed lines.
  Each `build:log:` message carries the line's number so the page can drop
  what it already has; this also fills the gap after a reconnect. Effort M.
- **L3. Progress against the usual duration.** A bar from `eta` on running
  rows and in the build header ("4m 12s of about 9m"); mark builds far over
  it, and flag a build past its usual duration that printed nothing for a
  while, as TeamCity does. Effort S.
- **L4. Status outside the page.** A status dot in the favicon on the build
  page (the title already has an emoji). Effort S.
- **L5. Notifications that follow the user.** "Notify me when builds matching
  this filter finish", kept in the browser, with the app subscribed to them
  on every page. Today the bell is per build, forgotten on reload, and fires
  only on the feed or that build's page. Effort M.
- **L6. Say when updates stop.** A "Reconnecting…" badge after a few seconds
  without the WebSocket. Effort S.

## Build page

- **B1. A header that says what happened.** A status-coloured band: "Failed
  at Run tests after 9m 46s (usually 8m 30s)", the F1 chips, who started it
  (recorded for analytics, not yet sent to the page), the F2 strip, previous
  and next build of the job, and labelled actions: Re-run (primary), Re-run
  with changes…, Abort while running; notify, hide logs and raw logs in "⋯".
  Compact and sticky, so status stays visible while a log follows. Effort M.
- **B2. Failure first.** Open the failed task, or the running one, at the end
  of its log. Jenkins Blue Ocean auto-opened whole failed logs and users
  complained that they pushed everything else away; opening at the end
  avoids that. "First error" heuristics don't work: in the logs looked at,
  the first line matching `Error` was harmless noise near the top. Effort S with the log viewer.
- **B3. Fold what passed.** Passed tasks before the failure fold into "✓ 12
  tasks passed · 31 s"; hooks fold by group ("After failure · 4 tasks");
  main tasks that never ran say "not run" in grey instead of showing the
  pending icon. Effort S.
- **B4. Log viewer.** See [Task logs](#task-logs).
- **B5. Summary first.** `$WAKE_SUMMARY` already shows above the tasks; the
  docs should show how a test task writes its failed tests and the value to
  re-run only them. Later: a "Re-run with these values" link, and Markdown
  rendered without raw HTML. Effort S, then M.
- **B6. Parameters folded.** "Parameters · 3 differ from defaults", the
  changed ones first. Re-run with changes opens the run form with this
  build's values. Effort S.
- **B7. History in reach.** The job's last 20 builds as a strip next to the
  title (`OutcomeStrip` from the analytics page), keys for previous and next
  build. Effort S.

## Task logs

Wanted: colours, links, search over the whole log, and no trouble at 10 MB
and above.

### Measurements

A generated log of 10 MB (129,455 lines, wakeci's time prefix, colour codes
on 17% of lines, URLs on 5%), in Chrome 1440×900 on a desktop. A blank page's
renderer uses 134 MB.

| 10 MB | One `<pre>` (finished task today) | Colours and links as HTML (a div per line) | Streaming today (running task) | CodeMirror 6, read-only |
|---|---|---|---|---|
| First paint | 0.4–1.2 s | 1.7–2.3 s | | 0.03 s, after 0.06 s to parse colours |
| Renderer memory | 533 MB | 886 MB | 1.2 GB | 321 MB |
| Elements | 8 | 322,532 | 8 | about 230 |
| Scroll to the middle | 25 ms | 80–150 ms | | 20 ms |
| Wrap on/off | 0.5–0.8 s | | | 34 ms |
| One 500 ms update | | | 50 ms at 1 MB, 387 ms at 10 MB | 33 ms for 100 lines, flat |

At 50 MB (644,310 lines): one `<pre>` paints in 2.9 s with 2 GB; CodeMirror
in 0.1 s after 0.4 s of parsing, with 529 MB.

Search over the 10 MB text: `indexOf` under 1 ms, a case-insensitive regex
5 ms (22 ms at 50 MB). CodeMirror's `SearchCursor` takes 0.4–0.6 s to count
all matches (2.3–2.8 s at 50 MB), so the viewer counts matches itself.

So the risk is not a finished plain log: Chrome draws 10 MB in one `<pre>`.
It is colours and links as ordinary elements (half a million nodes), and
streaming as `TaskItem` does it, which copies and lays out the whole log on
every update: at 10 MB 387 ms of every 500 ms, 1.2 GB and rising.

### Design

- **CodeMirror 6, read-only, as the viewer.** It renders only the visible
  lines, keeps selection and copy across lines that are off screen (a
  hand-written virtual list loses them), and has line numbers, wrapping and
  scrolling to a position. The job editor already ships it: the log viewer
  needs `@codemirror/view`, `state` and `search`, about 250 kB minified,
  shared with the editor's chunk, under the 500 kB limit.
- **The document is the visible text.** Colour codes are parsed once (60 ms
  for 10 MB) into spans per line; a view plugin turns the spans of the
  visible lines into decorations, so the cost does not grow with the log.
  Links come from a `MatchDecorator`, also only for visible lines: `http`
  and `https` only, opened with `rel="noopener noreferrer"`. Nothing from a
  log is ever inserted as HTML.
- **Search.** A search box of our own (Ctrl+F and `/` inside the viewer open
  it, as the browser can't find text that isn't drawn): `indexOf` or a regex
  over the document text, "37 of 2,633", next and previous via the editor's
  selection, and highlights as decorations of the visible matches only.
  "Search all tasks" uses the existing `GET /api/build/{id}/log/{task}?grep=`.
- **Line links and the time prefix.** Click a line number for `#L120`,
  shift-click for a range, with Copy lines. The `[ 9m46.704s] ` prefix moves
  to its own gutter that can be hidden.
- **Loading.** A finished task's log is fetched whole: full fetches are
  gzipped, and a real log compressed 5.7 times, so 10 MB is about 1.8 MB on
  the wire. Above 50 MB the viewer opens the last 10,000 lines (`?tail=`)
  and offers the full log; search then uses `grep`. A running task loads
  what is written, then appends streamed lines (L2), and follows the end
  only while the user is at the bottom; otherwise a "N new lines" pill.
- **Colours on the server.** Tasks run without a terminal, so most tools
  print no colours, and `ProcessLogEntry` strips the codes anyway.
  - Keep `task_N.log` plain: `WAKE_FAILED_TASK_LOG`, the raw log link, the
    `tail` and `grep` API, MCP and agents read it.
  - Write `task_N.ansi.log` beside it, line for line, keeping only SGR
    codes (colour and style), once a line of the task has any; before that
    the plain lines are the same. Other escape sequences are still removed.
  - An opt-in `color: true` on the job or task sets `FORCE_COLOR=1`,
    `CLICOLOR_FORCE=1` and `PY_COLORS=1`. Not on by default, since a script
    that parses a tool's output would get codes; not a PTY, which would merge
    stdout and stderr and change line endings.
  - Redact secrets in the visible text: today `redactSecrets` runs before
    `StripColor`, so a secret split by colour codes isn't matched, and
    stripping joins it again in the plain log. For the colour file, redact
    the visible text and keep the codes around it.
  - The 16 basic colours map to CSS variables chosen for contrast in light
    and dark; 256 colours through a table; 24-bit colours to the nearest of
    those.
- **Effort.** The viewer with search and links M–L; colours on the server M;
  line numbers in log messages S.

## Jobs page and run form

- **J1. The jobs page as a status board.** Each job's last 20 builds as a
  strip, last run, next scheduled run, "failing since …"; sort by recently
  failed; star to pin; group by name prefix or a `group:` in the job file.
  Concourse found users care most about which jobs fail and for how long.
  Effort M.
- **J2. A run form that explains itself.** Wider, changed values marked,
  the last values used per job remembered, and optional metadata kept apart
  from `params` so existing files don't change:

  ```yaml
  param_info:
    BROWSERS: {desc: "Browsers to test", options: ["firefox,chrome", firefox, chrome]}
    CLEANUP: {type: bool}
  ```

  Effort L.

## Phones and smaller things

- The header hides every page link on phones (`m l` classes); add a menu.
  The search row and some feed rows are wider than the screen.
- The status icon is a `<button>` that does nothing; use an image with a
  label.
- Pending and timed out are both purple, skipped is light green next to
  finished green. Pair colour with shape: grey outline for queued and not
  run, a clock for timed out.
- The "updates have been filtered" note is hidden on phones and doesn't
  offer to clear the filter.

## Order

| Phase | Items |
|---|---|
| 1 | F1, B2 and B3, L2, F3, phone menu |
| 2 | B1 and B6, the log viewer (B4), F4 and F5, L1, L3, L4, L6, J1 |
| 3 | F2 and B7, L5, colours on the server, J2 and B5, F6 |

## Sources

- GitHub Actions: [`run-name`](https://github.blog/changelog/2022-09-26-github-actions-dynamic-names-for-workflow-runs),
  [rendering large logs](https://github.blog/engineering/how-github-actions-renders-large-scale-logs),
  [failed steps expand, line links](https://docs.github.com/en/actions/how-tos/monitor-workflows/use-workflow-run-logs),
  [job summaries](https://github.blog/news-insights/product-news/supercharging-github-actions-with-job-summaries/),
  [notifications](https://docs.github.com/en/actions/concepts/workflows-and-actions/notifications-for-workflow-runs)
- Buildkite: [pipeline list](https://buildkite.com/docs/pipelines/dashboard-walkthrough),
  [My builds](https://buildkite.com/blog/my-builds),
  [build page](https://buildkite.com/resources/changelog/290-the-next-phase-of-the-build-page-experience),
  [favicons](https://buildkite.com/blog/build-status-favicons),
  [log sections](https://buildkite.com/docs/builds/managing-log-output)
- Concourse: [dashboard research](https://github.com/concourse/docs/blob/master/docs/blog/posts/2017/11/2017-11-27-designing-a-dashboard.md),
  [build page](https://github.com/concourse/docs/blob/master/docs/blog/posts/2017/10/2017-10-26-build-page-improvements.md)
- Jenkins Blue Ocean: [auto-opened failed logs](https://issues.jenkins.io/browse/JENKINS-56639)
- TeamCity: [progress and "probably hanging"](https://www.jetbrains.com/help/teamcity/rest/progressinfo.html)
- CircleCI: [denser pipeline list](https://circleci.com/changelog/pipelines-page-design-updates)
- Vercel: [logs of failed builds filter to errors](https://vercel.com/changelog/deployment-logs-filtering-now-available)
- NN/g: [preattentive attributes](https://www.nngroup.com/articles/dashboards-preattentive/);
  IBM Carbon: [status indicators](https://carbondesignsystem.com/patterns/status-indicator-pattern/)
