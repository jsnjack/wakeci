# UI roadmap: feed, live updates, build page, task logs, styles

Read this before changing the feed, labels, the build page, task logs, the
jobs page, the run form or the styles. It lists the changes planned for
them, the evidence behind each, and the order. When an item ships, delete it
here and name it (e.g. "F1") in the commit message.

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

- **F1. Labels first.** A label is a parameter the job marks as one: a name
  ending in `_LABEL`, or those listed in an optional `labels:` in the job
  file, in display order:

  ```yaml
  labels: [BRANCH, AUTHOR, COMMIT]  # without it, *_LABEL parameters are the labels
  ```

  Labels stay parameters: nothing new in the run API, the history or the
  audit log. The jobs list sends each job's labels next to its
  `defaultParams`. A row shows the label values first, then the other
  changed parameters, as grey text separated by dots, not boxes; the name
  and the full value show on hover, and a click adds the value to the
  search (F7). Named values (`branch main`) were tried and read as
  overloaded. Values at their default stay hidden, labels too: the only one
  seen was a placeholder. 40-character hex values are shortened to 7; long
  values are cut in the middle, keeping a leading ticket number. The build
  header (B1) and the run form (J2) show the same labels. Effort M.
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
- **F3. Two plain lines per build (decided).** Today's two lines in the same
  order, without the card, the boxes and the copy icons: the status icon,
  `#id` in grey and the job name on the first line, the F1 values on the
  second; duration and time on the right, without icons; thin lines between
  rows. About 60 px, 12 builds on 1440×900. The row is the link; Abort, Run
  again and Open in a new tab appear on hover and focus, only when they can
  act (today they show disabled), and in a "⋯" menu on phones. A running
  build has a thin progress line along its bottom edge. Queued builds say
  how long they have waited and their place in the queue. Effort M.
- **F4. Quick filters and saved searches.** A row of chips above the feed
  that write the existing `q`: Failed, Running and queued, Hide scheduled,
  several jobs, and the user's saved searches. "Save this search" names the
  current one. Where builds are started by another job or a shared
  credential, only a label says whose they are, so Mine is a saved search
  each person sets once (e.g. `+author=alice`). Kept in the browser first;
  per SSO user on the server later, so they follow the user between
  browsers. "Started by me" from the recorded trigger can come later; the
  feed doesn't send the trigger today. Effort M.
- **F7. Exact match in the search.** `NAME=value` matches a parameter equal
  to the value, and a label also by its shown name (`branch=main` finds
  `BRANCH_LABEL`). `NAME:value` keeps matching substrings, so links and
  scripts keep working, but `BRANCH_LABEL:main` also finds `main-fix` and
  `maintenance`. A chip click writes `+branch=main` instead of
  `+"BRANCH_LABEL:main"`; the `+` stays, since plain terms mean "any of" and
  Ctrl/Cmd+click must narrow the search. One list of queries and the builds they match,
  checked by the Go tests and an E2E test, keeps the server's and the
  browser's parsers from drifting apart again. Effort M.
- **F8. Help while typing.** The search box suggests label names, then
  values seen in the loaded builds; the active terms show as chips that can
  be removed one by one. Effort M.
- **F9. The build beside the feed (decided).** On a wide window a click on
  a row opens the build on the right, and the feed narrows to a list on the
  left (two short lines per build, the selected one highlighted), like an
  email app. The list keeps its search and scroll position; ↑ and ↓ move to
  the next build, Esc closes, and a button opens the build as its own page.
  The address names both, so a reload or a shared link restores them. Below
  the wide breakpoint a click opens the build page as today. The right half
  is the build page (B1–B3). Effort M.
- Not planned: labels added to a build after it ran ("flaky"). They would
  need storage per build, an audited route and access rules.

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
  what it already has; this also fills the gap after a reconnect. Today a
  reload of a running task replaces the text with the file, so lines waiting
  in the 500 ms buffer appear twice. The server sends a task's lines in
  batches (about every 100 ms), numbered from the first, in
  `CommandLogData.ID`, which nothing sets today: one message per line
  through the unbuffered hub disconnects a browser with 1,024 messages
  queued during a burst. Effort M.
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

Decided: one header, then the failure, then a plain task list (B1–B3). A
header with more in it (a status band, strips, counts) and folded task
groups were tried and read as overloaded.

- **B1. One header of three lines.** The status icon, the job name and its
  description, with labelled actions on the right: Run again, Abort while
  running, a bell to notify, and "⋯" for start now, hide logs and raw logs.
  Under it "#1204 · Failed after 7 min 46 s · started 11 min ago, 11:20
  UTC", then the F1 values and "All 11 parameters", which opens the full
  list (B6). No second card for the parameters. Who started the build, the
  usual duration and the F2 strip can join the second line later. Effort M.
- **B2. Failure first.** The failed task's log sits right under the header,
  in a box about 420 px high, opened at its end; its row in the list scrolls
  to it. A running task opens in its place, in the list, following its end.
  Jenkins Blue Ocean auto-opened whole failed logs and users complained
  that they pushed everything else away; a box opened at the end avoids
  that. "First error" heuristics don't work: in the logs looked at, the first
  line matching `Error` was harmless noise near the top, and the failing
  test was named about 40 lines above the end. Search in the log (B4) finds
  it. Effort S with the log viewer.
- **B3. A plain task list.** Every task that ran, and every main task, as a
  row of about 34 px: status, name, duration, no buttons; a click opens its
  log under the row, with raw log and reload in the log's bar. Hooks sit
  under small headings: Before start, After failure, After abort, After
  success, Always. Main tasks that never ran say "not run" in grey instead
  of showing the pending icon. Effort S.
- **B4. Log viewer.** See [Task logs](#task-logs).
- **B5. Summary first.** `$WAKE_SUMMARY` already shows above the tasks; the
  docs should show how a test task writes its failed tests and the value to
  re-run only them. Later: a "Re-run with these values" link, and Markdown
  rendered without raw HTML. Effort S, then M.
- **B6. All parameters on demand.** "All 11 parameters" opens a list of
  names and values, the changed ones first. Re-run with changes opens the
  run form with this build's values. Effort S.
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

A hand-written viewer in the prototype kept the text in 256 KB pieces with
a typed-array line index, drew 200-line chunks only near the visible part,
and worked out colours, links and search hits only for drawn lines. An 8 MB
coloured log was indexed in 5 ms and its end drawn in about 40 ms, with 400
lines in the page; scrolling it top to bottom held 60 frames a second;
searching all of it took 12–22 ms; 10 MB streamed at 1 MB/s (7,000 lines a
second) averaged 17–18 ms a frame, the worst 33–50 ms. It is about 10 kB
instead of CodeMirror's 250 kB, but a selection dragged past the drawn
chunks loses its text, so CodeMirror stays the engine and the prototype
fixes the look and behaviour.

### Design

- **CodeMirror 6, read-only, as the viewer.** It renders only the visible
  lines, keeps selection and copy across lines that are off screen (a
  hand-written virtual list loses them), and has line numbers, wrapping and
  scrolling to a position. The job editor already ships it: the log viewer
  needs `@codemirror/view`, `state` and `search`, about 250 kB minified,
  shared with the editor's chunk, under the 500 kB limit. Import them
  directly, not through `vue-codemirror`'s `basicSetup` (a 428 kB chunk with
  autocompletion and lint), and add `state` and `search` to `package.json`;
  today they come only through other packages. The editor view stays out of
  Vue's reactivity, and streamed lines are appended in one transaction per
  animation frame, never by rebuilding the text.
- **The document is the visible text.** Colour codes are parsed once (60 ms
  for 10 MB) into spans per line; a view plugin turns the spans of the
  visible lines into decorations, so the cost does not grow with the log.
  Links come from a `MatchDecorator`, also only for visible lines: `http`
  and `https` only, opened with `rel="noopener noreferrer"`. Terminal
  hyperlinks (OSC 8, printed by test reporters and `ls --hyperlink`) become
  links too, under the same rule. A line redrawn with carriage returns (a
  progress bar) shows only its last state; cursor movement and other escape
  sequences are dropped. Nothing from a log is ever inserted as HTML.
- **Search.** A search box of our own (Ctrl+F and `/` inside the viewer open
  it, as the browser can't find text that isn't drawn) with the rules of
  `GET /api/build/{id}/log/{task}?grep=`: text, not a regex, ignoring case
  (an Aa toggle, and `case=1` for `grep`), in each line after its time
  prefix. A regex would run on every streamed batch in the page, and a slow
  one freezes it. "37 of 2,633", next and previous via the editor's
  selection, and highlights as decorations of the visible matches only.
  - **While the task runs (decided).** A line never changes once it appears:
    go-cmd hands a line over at its line break, and `ProcessLogEntry` keeps
    the last state of a `\r` redraw. So each streamed batch is searched on
    its own; the count grows and the current match stays. A jump to a match
    pauses following, which today only a wheel or key scroll does (it sets
    `readingAbove`); new lines and matches wait behind the pill ("214 new
    lines · 2 new matches").
  - **Only matching lines (decided),** a toggle, like `tail -f | grep`: the
    matching lines with their own line numbers and "⋯ 245 lines" between
    them, new matches appended as they arrive. A click on a line number
    opens the full log at that line.
  - **Search all tasks** lists the matches per task: finished tasks from
    `grep`, the running task from the viewer's own text with new matches
    added as they arrive, later tasks as "not started". `grep` stops at
    1,000 matches, shown as "1,000+".
  - **After a reconnect** the page fetches the log again; the search keeps
    its query, counts again and keeps the current match by its line number.
- **Line links and the time prefix.** Click a line number to put the task
  and the line in the address (`#t13-L120`), shift-click for a range
  (`#t13-L120-L125`), with Copy lines; opening such a link opens that task
  scrolled to the lines, marked. The `[ 9m46.704s] ` prefix is drawn in grey,
  and can move to its own gutter that can be hidden.
- **Wrapping.** On by default, as today; off keeps each line on one row and
  the line numbers in place while scrolling sideways.
- **Loading.** A finished task's log is fetched whole: full fetches are
  gzipped, and a real log compressed 5.7 times, so 10 MB is about 1.8 MB on
  the wire. Above 50 MB the viewer opens the last 10,000 lines (`?tail=`)
  and offers the full log; search then uses `grep`. A running task loads
  what is written, then appends streamed lines (L2), and follows the end
  only while the user is at the bottom; otherwise a "N new lines" pill. A
  running task that grows past 50 MB while open drops its oldest lines and
  says so; the search then covers what it keeps and offers the whole log
  through `grep`.
- **Colours on the server.** Tasks run without a terminal, so most tools
  print no colours, and `ProcessLogEntry` strips the codes anyway.
  - Keep `task_N.log` plain: `WAKE_FAILED_TASK_LOG`, the raw log link, the
    `tail` and `grep` API, MCP and agents read it.
  - Write `task_N.ansi.log` beside it, line for line, keeping only SGR
    codes (colour and style), once a line of the task has any; before that
    the plain lines are the same. Other escape sequences are still removed.
  - Jobs opt in (decided): `color: true` on the job or task sets
    `FORCE_COLOR=1`, `CLICOLOR_FORCE=1` and `PY_COLORS=1`, and a job may set
    them, or a tool's `--color=always`, itself. Not on by default, since a
    script that parses a tool's output would get codes; not a PTY, which
    would merge stdout and stderr and change line endings.
  - Redact secrets in the visible text: today `redactSecrets` runs before
    `StripColor`, so a secret split by colour codes isn't matched, and
    stripping joins it again in the plain log. For the colour file, redact
    the visible text and keep the codes around it.
  - The 16 basic colours map to CSS variables, one palette for light and
    one for dark, each readable on its log background. 256 and 24-bit
    colours keep the program's colour, moved toward the text colour until
    it has 4.5:1 contrast with the background (white text on the light
    theme turns grey), cached per colour. Background colours are drawn at
    about a quarter strength, so text on them stays readable in both
    themes.
- **Tests.** `secrets.spec.js` matches the `.log-line` text with the time
  prefix inline; with the prefix in a gutter it reads the viewer's text
  instead. Tests that look for a line in the page text pass while short logs
  are drawn whole.
- **Effort.** The viewer with search and links M–L; colours on the server M;
  line numbers in log messages S.

## Jobs page and run form

- **J1. The jobs page as a status board.** Each job's last 20 builds as a
  strip, last run, next scheduled run, "failing since …"; sort by recently
  failed; star to pin; group by name prefix or a `group:` in the job file.
  Concourse found users care most about which jobs fail and for how long.
  The search goes in the URL and survives a reconnect, and names sort as
  people expect, not in Bolt's byte order. Effort M.
- **J2. A run form that explains itself.** Wider, changed values marked,
  the last values used per job remembered, and optional metadata kept apart
  from `params` so existing files don't change:

  ```yaml
  param_info:
    BROWSERS: {desc: "Browsers to test", options: ["firefox,chrome", firefox, chrome]}
    CLEANUP: {type: bool}
  ```

  Labels come first. An empty value means the job's default (`RunJob`), so
  a parameter with a default can't be run empty; the form says so. Effort L.

## Styles

BeerCSS 5.0.3 shapes most of the UI, much of it without the templates
saying so:

- 36 of the 43 components use its classes (835 uses of 82 classes).
- Its reset gives every element `position: relative`, no margin or padding
  and `border-radius: inherit`. `article`, `nav`, `p`, `pre`, `table`,
  headings and others get a 1rem top margin after a sibling, which
  `no-margin` cancels 31 times.
- Its script, loaded with the CSS, watches `body` and runs its setup again
  180 ms after DOM changes. Floating field labels work only because it sets
  `placeholder=" "` on inputs. The run dialog opens with its `ui()`, the
  other ten dialogs with the `active` class.
- Five components carry comments working around it, and `FeedItem` needs
  `!important`. `medium-padding` and `medium-margin` (10 uses) don't exist
  in 5.0.3.
- It adds 88 kB of CSS and 91 kB of shape images the app never uses, 19 kB
  of JavaScript, and three icon fonts, 1.43 MB in the binary; browsers fetch
  only Outlined (464 kB) for the 67 icons in use.
- The 36 theme colours in `main.css` equal its defaults, so they carry over
  unchanged.

It goes in favour of a stylesheet of our own, one page at a time. Not
Tailwind, which rewrites every class use and adds a build plugin, nor a
component kit, which is another lock-in with larger chunks.

- **S1. Groundwork.** Screenshot baselines (`toHaveScreenshot`) of the feed,
  a failed and a running build, the jobs page, the run form, settings and
  analytics, at 1440, 1024 and 390 px, light and dark, times masked: each
  later PR shows what it changed on screen. The 24 E2E selectors that depend
  on markup (`nav a[href=…]`, `header h6`, `dialog.active`, `progress`,
  `.row`) move to test IDs. `tokens.css` (colours, spacing, radius, type
  scale) and `base.css`; BeerCSS imported into `@layer beer` under our
  `@layer app`, so our rules win without `!important`. Effort S–M.
- **S2. Components.** About 40 styles replace what the app uses: rows and
  grids with `gap` (no 12-column classes, no automatic margins); `.btn` in
  four kinds and two sizes; one dialog component on the native `<dialog>`
  with `showModal()`; labels above fields, which need no script; a segmented
  control and tabs; tooltips in our CSS, with `aria-label` on icon-only
  buttons; notifications; three named breakpoints. Effort M.
- **S3. Page by page,** each with that page's items so it changes once: the
  feed with F1, F3 and F7; the build page with B1–B4 and B6; the jobs page
  with J1; the run form with J2; then settings, keys and groups, analytics,
  help and login. Effort M per page.
- **S4. Remove BeerCSS** once no page uses it, and ship an icon font with
  only the icons in use, generated by a script, with a test that every icon
  name in `src` is in it. Effort S.

The look (decided): Material's colours at the density of a developer tool.

- **Colour.** Material's tonal palette from wakeci's purple, so the 36 theme
  colours in `main.css` carry over and hover, selection, light and dark all
  follow from them. Dark follows the system unless the user picks one.
  Colour means status only, always with a shape; one accent colour.
- **Shape and density.** 12 px corners on containers, 8 to 10 px on
  controls, pills only for navigation; thin dividers instead of cards and
  shadows; text about 15 px. Material's own components are sized for touch
  (40 px chips, 48 px targets), which is why today's feed fits 8 builds.
- **Type.** Roboto Flex cut to Latin letters and the weight axis, or the
  system font. Open: the full Roboto Flex file is 1.8 MB, and the cut one
  is not measured yet.
- Tried and not chosen: Material 3 Expressive (28 px corners, list items as
  separate tiles, bolder titles) fitted 10 builds and looked playful; a
  neutral grey style like GitHub's fitted 13 but lost the wakeci look.

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
| 1 | S1 and S2; the feed on the new styles (S3) with F1, F7 and F3; phone menu |
| 2 | The log viewer (B4) with L2, colours on the server; the build page on the new styles with B1–B3 and B6; then the build beside the feed (F9) |
| 3 | F4 and F8, J1, J2 |
| 4 | The other pages on the new styles, S4 |
| Later | F2, L1, L3–L6, B5, B7 |

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
