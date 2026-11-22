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

- **F4, later.** Saved searches are kept in the browser (`FeedSearch.vue`);
  per SSO user on the server they would follow the user between browsers.
  "Started by me" could come from the recorded trigger, which the feed
  doesn't send. Effort M.
- Not planned: labels added to a build after it ran ("flaky"). They would
  need storage per build, an audited route and access rules.

## Build page

The page has one header, then the failure, then a plain task list, with
logs in the viewer (docs/logs.md). A header with more in it (a status
band, strips, counts) and folded task groups were tried and read as
overloaded.

- **B5, later.** The job docs show how a test task writes its failed tests
  to `$WAKE_SUMMARY` with the value that runs only them again. Next: a
  "Re-run with these values" link, and Markdown rendered without raw HTML.
  Effort M.
- **B7. History in reach.** The job's last 20 builds as a strip next to the
  title (`OutcomeStrip` from the analytics page), keys for previous and next
  build. Effort S.

## Order

| Phase | Items |
|---|---|
| Later | F4 later, B5 later, B7 |

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
