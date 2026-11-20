# Task logs

Read this before changing how tasks write their logs, the log route, log
messages, or the log viewer on the build page.

## Files

A task writes `task_N.log` in its build's wakespace: each line of its
output after the time since the task started (`[    1.200s] `), with the
lines wakeci adds (`> Running command: …`) starting with `>`. Secrets are
redacted in the visible text, colour codes and other escape sequences are
removed, and a line redrawn with carriage returns keeps its last state.
`WAKE_FAILED_TASK_LOG`, the raw log link (`/storage/build/{id}/task_N.log`),
`tail`, `grep`, MCP and agents read this file, so it stays plain.

A task whose output has colours also gets `task_N.ansi.log`: the same
lines, with the colour and style codes (SGR) and terminal hyperlinks
(OSC 8) of its output (`colors.go`). It starts at the first line with
any, with the lines before it copied in, so the two files stay line for
line. Tasks run without a terminal, so most tools print colours only when
asked: `color: true` on a job or task sets `FORCE_COLOR=1`,
`CLICOLOR_FORCE=1` and `PY_COLORS=1`, before the params and the task's
`env`. It is off by default, since a script that parses a tool's output
would get the codes.

`taskLogWriter` buffers both files and counts the lines it wrote. A read of
a running task's log writes the buffer out first (`FlushLogs`, which only
locks the writer), so the file has every line sent so far.

## Messages

Lines go to the browsers in `build:log:{id}` messages, one task's lines a
message, sent together after up to 100 ms or 64 KB (`logBatch`): `id` is
the number of the first line in the task's log, `data` the plain lines and
`ansi`, when a line had colours, the same lines with them. A task's last
lines go out before its status changes. One message a line used to fill a
client's queue of 1,024 and disconnect it.

## The log route

`GET /api/build/{id}/log/{task}` returns the whole log (Range works), the
colour log with `ansi=1` if there is one, the last lines with `tail`, or
the lines that contain a text with `grep`, ignoring case unless `case=1`
and the time each line starts with, at most 1,000.

## The viewer

`LogViewer.vue` shows a log with CodeMirror 6, read-only: it draws only
the lines in view, and keeps a selection across lines that aren't drawn.
The document is the visible text. `log/ansi.js` parses the colours once,
into spans per line, and the viewer turns those of the lines in view into
decorations, with the time prefix in grey, web links (http and https, and
OSC 8 links to them) opened in a new tab without the opener, search
matches and marked lines. The 16 basic colours are CSS variables, a
palette for light and one for dark (`assets/log.css`); others keep the
program's colour, moved toward the text colour until it has 4.5:1 contrast
with the background (`log/colors.js`). Nothing from a log is inserted as
HTML.

It subscribes before it reads the log, then appends the lines that arrive,
once a frame: those it already has are dropped by their numbers, and a gap,
such as after a reconnect, reads the log again. It follows the end while
the user is there; scrolling up stops that and collects new lines behind a
pill. A log over 50 MB opens with its last 10,000 lines (`tail`, without
colours or line numbers), and a running task's log that grows past 50 MB
drops its oldest lines and says so.

The search box (Ctrl+F or `/`, since the browser can't find lines that
aren't drawn) has `grep`'s rules: text, not a regex, ignoring case unless
Aa is on, after the time prefix. A line never changes once it appears, so
new lines are searched on their own and the current match stays.
"Only matching lines" hides the others behind "⋯ N lines". A jump to a
match stops following. A click on a line number puts the task and line in
the address (`#t13-L120`), Shift+click a range (`#t13-L120-L125`), with
Copy lines; such an address opens the log at those lines.

"Search all tasks" on the build page asks `grep` for each task that
started, and a match opens that task's log at its line.

## Why CodeMirror, and these limits

A generated log of 10 MB (129,455 lines, wakeci's time prefix, colour codes
on 17% of lines, URLs on 5%), in Chrome 1440×900 on a desktop. A blank page's
renderer uses 134 MB.

| 10 MB | One `<pre>` (the old finished task) | Colours and links as HTML (a div per line) | Old streaming (running task) | CodeMirror 6, read-only |
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
streaming as the old `TaskItem` did it, which copied and laid out the whole
log on every update: at 10 MB 387 ms of every 500 ms, 1.2 GB and rising.

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
