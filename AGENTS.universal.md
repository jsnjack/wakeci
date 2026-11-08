# Universal Project Conventions

Run `make check` (or the project's equivalent validation gate) after every
change. It must pass before reporting the task as complete.

---

## Work patterns

- One concern per change. Don't mix refactoring with feature work.
- **Minimal footprint.** Change only what is needed to satisfy the requirement.
  Don't refactor nearby code, reorganize files, or improve unrelated things
  unless explicitly asked.
- **Clarify before assuming.** When requirements are ambiguous or conflicting,
  ask — don't guess and proceed.
- **Present options.** When a task has multiple valid solutions, list all of
  them with a short description and pros/cons for each. Ask the user to choose
  before doing any implementation.
- **Plan in concepts, not code vocabulary.** When describing a plan, use plain
  language — not variable names, function names, or pseudocode. Write "if
  output is suppressed" not "if `isSilent` is true"; write "the retry limit"
  not "`maxRetries`". Code names are implementation details that belong in the
  code, not in the reasoning.
- **Keep the build current.** After a change compiles, run `make build` —
  don't wait for the full `make check` gate at the end. The user should
  always have an up-to-date binary on disk to try, not just a change that
  passed tests.
- Never report work as done until all requirements are met and `make check`
  passes. If requirements cannot be met, say so explicitly.
- When something is unclear, read the existing code first — match its patterns.
- **Keep `AGENTS.md` current and short.** After any change that affects
  project structure, architecture, patterns, or design decisions, update the
  documentation to reflect the new state: `AGENTS.md`, or the `docs/` file
  for that area (see [AGENTS.md size](#agentsmd-size)). A stale `AGENTS.md`
  is worse than none, and so is one too long to be read whole.

---

## Build & validate

Every project exposes a single validation command (typically `make check`) that
runs in order: format → vet → build → test → lint → AGENTS.md size. All steps
must pass.

Missing tools print an install command and exit — never auto-install silently.

```bash
make check      # full validation gate — run after every change
make test       # tests only
make build      # compile only
make standards  # refresh all applicable convention files from the standards repo (if present)
```

---

## AGENTS.md size

Every agent session loads `AGENTS.md` whole, and Codex reads only its first
32 KiB and silently drops the rest. So `AGENTS.md` stays under 30,000 bytes,
and `make check` fails when it is over (`make agents-size`).

- `AGENTS.md` holds what every session needs: the overview, the package map
  (one line per package, not per file), the rules that cross packages, the
  commands, and the traps.
- The detail of a feature or flow goes in `docs/<area>.md`. `AGENTS.md` lists
  those files in a "Where the details are" table that says which file to read
  before working on which area.
- The reason for a particular line (the incident, the measurement, the
  alternative that failed) goes in a comment beside that line or in the
  commit message, not in `AGENTS.md`.
- When a change alters a flow, update that flow's file in `docs/`. Change
  `AGENTS.md` only when the package map, a rule that crosses packages, a
  command, or a trap changes.
- When `AGENTS.md` is over the limit, move whole sections to `docs/` word for
  word, so nothing is lost, and leave a short summary with a link in their
  place. Never delete a rule to make it fit.

---

## Logging

Two debug flags on every command that produces observable behaviour:

- `--debug` / `-d` — verbose output to **stderr**. Human-readable. State
  changes, requests, responses. For interactive debugging.
- `--trace` — maximum detail written to `/tmp/<binary>.log`, **truncated on
  every start**. Wire data, every state transition. Designed for agent
  self-diagnosis — when something breaks, run with `--trace` and read the log.

Default (no flags): warnings and errors only.

The two flags are independent and composable.

**TUI applications:** the TUI owns the terminal. Never write logs to stderr —
it corrupts the UI. Route all diagnostic output to the trace file. Surface
errors through the UI itself.

---

## Code quality

- **Thin entry point.** CLI/main handles argument parsing, configuration
  loading, wiring, and startup only. No business logic.
- **Single responsibility.** Each module/package does one thing. Name it after
  what it does, not what uses it.
- **No hidden coupling.** Cross-module side effects are expressed as explicit
  callbacks or interfaces — never rely on shared global state or reach into
  another module's internals.
- **Errors carry context.** Every error returned or logged includes enough
  context to understand where it came from and why. Never propagate a bare
  error silently.
- **Fail fast on setup, recover gracefully at runtime.** Startup errors should
  exit immediately. Errors in long-running background workers (watchers,
  servers, queues) should log and continue — never crash a running service over
  a single bad event.
- **No new dependencies without justification.** Prefer the standard library
  or built-in tooling. Only introduce a third-party dependency when the
  justification is explicit and agreed.
- **Flag breaking changes.** If a change alters existing behaviour (API,
  config format, output format, CLI flags), say so before implementing it.

---

## Code style

- **Consistency over preference.** Match the style of the surrounding code.
  When adding to an existing pattern, extend it — don't introduce a second
  pattern.
- **Names describe what, not how.** Function and variable names describe their
  purpose, not their implementation.
- **Public symbols have documentation.** Every exported type, function, and
  constant has a doc comment.
- **No magic values.** Any literal (string, number, identifier) used in more
  than one place gets a named constant.
- **No dead code.** Remove unused code. If something is temporarily disabled,
  replace it with a TODO comment explaining why and what needs to happen.
- **Comments earn their place.** Default to no comment. Write one only when it
  explains a fact that isn't obvious from the code itself. Keep concise "why"
  comments that explain the origin of a non-obvious value, a security or sandbox
  invariant, a compatibility quirk or workaround, an easy way to break the code,
  an invisible rule the code depends on, or an upstream reference (ticket,
  advisory ID, or URL). Prefer one line; use more only when one line would omit
  necessary context.
  Never write AI diary or proof-of-work notes, including verification reports.
  Never narrate mechanical control flow, restate what code or configuration does,
  merely repeat a name or type signature, add reassurance ("safe", "read-only",
  "no changes"), or put process and opinions in code (those belong in
  documentation).
  Required public-symbol documentation and actionable TODOs are exceptions, but
  they must add information beyond the symbol name or signature. Match how much
  the surrounding file comments; when unsure, leave it out.

---

## Writing style

Applies to all prose: docs, READMEs, commit messages, UI copy, and summaries.

Plain, calm, factual writing in complete sentences. Specifically banned:

- Sentence fragments for punch ("Not a web page in a frame." "Fast. Local.
  Yours.")
- Bold-lead marketing bullets ("**AI on your terms.** Any endpoint works…")
- Stating the obvious or overexplaining (don't explain what an RPM is, don't
  reassure twice that a feature is optional)
- Negative or edgy framing, taglines, and competitor comparisons
- Jargon where an ordinary word exists

Good example of the register: "Mail is stored in a local database, so search
is instant and reading works offline."

---

## Commits

Every commit message starts with a single-letter prefix that drives automatic
versioning ([jsnjack/monova](https://github.com/jsnjack/monova)):

- `M` — major change (breaking).
- `m` — minor change (backward-compatible feature).
- `p` — patch (backward-compatible fix).

The prefix is the first character of the subject line, followed by a space:

```
m Add --trace flag to the server command
p Fix panic when the config file is empty
```

---

## Testing

Tests exist to catch regressions — a test that doesn't fail when behaviour
breaks has no value.

- Tests live next to the code they test.
- A good test is readable, deterministic, and isolated — it should be
  understandable without reading the implementation, produce the same result on
  every run, and not depend on external state or other tests.
- Tests are structured as named cases run independently — one assertion failure
  should not block others.
- Every new exported function gets a test. Every bug fix gets a regression test.

### GUI tests

GUI tests, smoke checks and screenshot scripts run in the background, so the
user is never interrupted: they never open a window, take the focus, play a
sound, open a link or change a setting on the user's desktop. This covers
temporary and ad hoc launchers too.

- Every project with GUI tests keeps one wrapper script, conventionally
  `scripts/headless-gui.sh`. Copy
  [headless-gui.sh](https://github.com/jsnjack/standards/blob/master/headless-gui.sh)
  from the standards repository and adapt it. Makefile targets and every ad
  hoc GUI command run through it, for example
  `scripts/headless-gui.sh go test ./internal/ui -run TestX -count=1`. Never
  type `dbus-run-session` by hand.
- The wrapper gives the command a private display (Xvfb or a headless
  compositor), unsets the desktop's `WAYLAND_DISPLAY` and `WAYLAND_SOCKET`,
  and starts a private D-Bus session bus with `dbus-run-session`.
- Before the bus starts, the wrapper sets `XDG_RUNTIME_DIR` to a fresh
  temporary directory with mode `0700`, one per session. A private bus or Xvfb
  alone is not enough: a document portal started on a bus that shares
  `/run/user/$UID` unmounts the desktop's `doc` filesystem, and every Flatpak
  app then fails with `bwrap: Can't find source path /run/user/$UID/doc/by-app/…`
  ([upstream bug](https://github.com/flatpak/xdg-desktop-portal/issues/2140)).
  When the command needs the desktop's runtime directory (rootless Podman), the
  wrapper instead starts the bus with a configuration that lists no service
  directories, so no portal can start.
- The wrapper puts stub `xdg-open` and browser launchers first on `PATH`, uses
  settings that cannot change the desktop's (`GSETTINGS_BACKEND=memory` or a
  private keyfile), and never touches the desktop's clipboard.
- The wrapper cleans up the test's children, private mounts and temporary
  directory on success and failure. It fails the run when `/run/user/$UID/doc`
  was mounted before the run and is gone after it.
- A run that needs the desktop's session bus, for example for the keyring,
  starts no bus of its own and still uses a private display with
  `WAYLAND_DISPLAY` unset.
- Browser tests run headless. A headed run is an explicit opt-in target, never
  the default.
- Never unmount the desktop's document portal or restart its services as part
  of a test. If Flatpak apps stop starting, restore the mount with
  `systemctl --user restart xdg-document-portal.service`, then find the run
  that removed it.

---

## Never

- Never skip `make check` before reporting the task as complete.
- Never run a GUI test or app on the user's desktop, and never start
  `dbus-run-session` by hand; use the project's GUI wrapper.
- Never swallow errors silently — always log or return with context.
- Never log to stderr in TUI applications.
- Never auto-install missing tools — print the install command and exit.
- Never leave commented-out code in the codebase.
- Never introduce a second pattern when one already exists.
- Never change code outside the scope of the current task.
- Never add a dependency without explicit justification and agreement.
- Never change existing behaviour silently — always flag it first.
- Never commit or push on behalf of the user unless they explicitly ask for it.
- Never write a commit message without an `M`/`m`/`p` version prefix.
