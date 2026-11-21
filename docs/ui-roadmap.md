# UI roadmap: feed, live updates, build page, task logs

Read this before changing the feed, labels, live updates, the build page or
task logs. It lists the changes planned for them, the evidence behind each,
and the order. When an item ships, delete it here and name it (e.g. "F1") in
the commit message. How the styles work: [styles.md](styles.md).

## Evidence

- **Real use.** Two days of a busy installation in November 2026: 1,815
  builds, up to 146 an hour (2.4 a minute). A job that starts eight test
  jobs for each pushed commit, and those eight jobs, made 68% of the builds;
  a commit's builds (median 10) were spread over 28 feed rows, between those
  of other commits. 19% were scheduled or maintenance builds, 98% of which
  passed.
- **Nine days later** (all 6,011 builds the installation keeps): 738 builds
  a day (median), up to 165 an hour and 28 running at once. 69% of builds
  belonged to a commit's group (10 builds, median), spread over 21 feed rows
  (median; 107 at the 90th percentile); half of those commits had a failed
  or timed-out build. 6% were aborted, all superseded by a newer push.
- **What the feed showed.** Since parameters at their default are hidden,
  a test build shows a commit hash cut to 27 characters, the branch and the
  author, none with its name: in 77% of rows with chips the first is a
  40-character hash. Branch names are 26 characters (median; 40 at the 90th
  percentile), most starting with a ticket number.
- **Labels.** Jobs name the parameters that say what a build is about
  `*_LABEL`: branch, author and region, on 75% of builds. 5.5% of those hid
  a label: mostly a branch at its default, the placeholder `unknown`; 1% by
  position. The other 25% were routine jobs without labels, told apart by a
  parameter such as a server pattern. wakeci itself has no notion of labels.
- **Who starts builds.** Of the builds whose trigger was recorded, API keys
  started 75% and schedules 15%, the admin password 7% and SSO users 2%, so
  "my builds" can only come from a label such as the author.
- **Failed builds.** The failed task was row 14 of about 20 (median), below
  the fold. Failing task logs were 1,300 to 24,000 lines (110 KB to 2.4 MB),
  with the failed tests and the totals in the last 20 lines.
- **Screens.** On 1440×900, 7 to 8 feed rows fit (80 to 96 px each). On a
  390 px phone the header hides every link except Log out.
- **Layouts tried.** Five feed and five build-page layouts were drawn on
  real builds, and the chosen ones built as a working prototype. With two
  plain lines per build the feed shows 12 builds on 1440×900 instead of 8; a
  denser one-line table showed 17 but cut long branch names. On the build
  page the last line of the failed log was 672 px down instead of behind a
  click. The prototype's log viewer, written by hand, opened an 8 MB
  coloured log (57,600 lines) in about 65 ms and streamed 10 MB at 1 MB/s
  with no frame over 50 ms (Chrome on a desktop).
- **Other tools.** Sources at the end.

## Feed

- **F2. Builds under the build that started them (decided).** A job that
  starts others adds `WAKE_PARENT=$WAKE_BUILD_ID` to the run call: not
  `parent`, which a job may have as a parameter. The started build gets
  `WAKE_PARENT_ID`. The parent must still be in the queue (otherwise 400,
  "build 5115 has ended") and readable by the caller (otherwise as if it
  didn't exist), so no one can hang builds under an old build, and loops
  can't happen. Each build stores its parent and its root in the history,
  stats and queue records, with a `parent` audit attribute, MCP argument and
  API docs.
  - **The feed** shows the root's row, then every build of the group on a
    short line below it, always, so the failure needs no click. The lines
    form a tree in the order the builds started, indented 22 px per level,
    up to 4 levels (2 on phones); deeper builds stay at the last level with
    "via #5210". The root row is running while builds run, with "1 failed"
    as soon as one fails, and at the end shows the worst of them. A build
    that started more than 10 lists its failed, running and queued ones,
    then a "+26 passed" line that opens the rest. The group sits at the
    root's place: the feed pages by the root's ID, and a bucket from each
    root to its members reads them without scanning the history.
  - **The build page** of a member says "Part of #5115 push" with the
    group's strip, and `[` and `]` move between siblings. The root's page
    lists the builds it started.
  - Run again on a member keeps its parent, copied by the server, so the
    parent may have ended; the tree shows each job's latest attempt ("2
    attempts" on hover). Abort on the root row aborts the whole group: 6% of
    builds were aborted one by one when a newer push superseded them. A
    search shows a group when any of its builds matches. A member's live
    update carries its root. An SSO user sees the members they may read and
    "+3 you can't see"; members whose root they can't read, or whose root
    was cleaned up, show as their own rows.
  - Explicit rather than guessed: grouping by an equal commit value would
    merge re-runs and deploys and miss jobs without one. Effort M to record
    the parent and show it on the build page, which jobs can adopt first;
    M–L for the feed.
- **F4, later.** Saved searches are kept in the browser (`FeedSearch.vue`);
  per SSO user on the server they would follow the user between browsers.
  "Started by me" could come from the recorded trigger, which the feed
  doesn't send. Effort M.
- Not planned: labels added to a build after it ran ("flaky"). They would
  need storage per build, an audited route and access rules.

## Live updates

- **L1. Keep the list still while the user reads.** Update rows in place
  with a short highlight on a status change. While the user has scrolled down
  or hovers a row, collect new builds behind a "3 new builds" pill; insert
  them on click or at the top. Today `FeedView.applyUpdate` adds and
  re-sorts at once. Effort S.
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

The page has one header, then the failure, then a plain task list, with
logs in the viewer (docs/logs.md). A header with more in it (a status
band, strips, counts) and folded task groups were tried and read as
overloaded.

- **B5. Summary first.** `$WAKE_SUMMARY` already shows above the tasks; the
  docs should show how a test task writes its failed tests and the value to
  re-run only them. Later: a "Re-run with these values" link, and Markdown
  rendered without raw HTML. Effort S, then M.
- **B7. History in reach.** The job's last 20 builds as a strip next to the
  title (`OutcomeStrip` from the analytics page), keys for previous and next
  build. Effort S.

## Order

| Phase | Items |
|---|---|
| Later | F2, F4 later, L1, L3–L6, B5, B7 |

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
