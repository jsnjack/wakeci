# Stopping, restarting and updating

Read this before changing how wakeci stops, drains and restarts, restores
queued builds, saves sessions, or updates itself. AGENTS.md keeps a summary.

## Flow

**Stop and restart**: SIGTERM/SIGINT, or `HandleUpdatePost`, calls
`Queue.BeginStop`, so no queued build starts again; HTTP keeps serving and new
builds are queued. `drainQueue` waits for the running builds until `abortAt`
(`draintimeout`), then aborts them, also the hooks of builds aborted while
queued, with a note in their logs, and waits `stopHookTimeout` for their
hooks; each further signal skips one wait. Every signal goes through
`forwardStopSignals`, which records it on the queue (`noteStopSignal`): it
cancels a restart an update planned, since `systemctl stop` or a reboot waits
for the process to end. Then the server shuts down, cron stops, `on_pending`
tasks of queued builds get `stopPendingTasksTimeout` (a further signal ends
this wait too), `stopTasks` aborts every task still running and starts no more
task or build (`stopTasksTimeout`; a build `RunJob` is still adding is queued
first or refused, see `addNewBuild`), failed session deletions are retried,
and the DB closes. Only then does `run` stop handling signals, so a further
one can't cut these steps short; one after that ends the process. After an
update `main` re-execs the new binary with `syscall.Exec` (same PID) unless a
signal cancelled it. If tasks still ran that `stopTasks` couldn't stop,
`exitPlan` makes it exit with status 1 instead, so a service manager ends them
and starts the new binary. Sessions are saved, so open pages reconnect;
`App.vue` fetches `/api/queue/` on every connection and reloads when its
`version` changed; it ignores the answer if a `queue:status` message arrived
while the request ran.

**Update**: `HandleUpdatePost` asks GitHub for the latest release, refuses
unless it is the requested, newer version and `updateProblem` is empty,
downloads it next to the binary, checks the sha256 digest GitHub lists and
that `--version` prints the version, hard-links the old binary to
`<binary>.previous` (refusing if it can't), renames the new one over it, then
`BeginStop`.

**Database backup**: the first time a version opens the database, at startup
or in `--import-settings`, `backupForNewVersion` writes a copy next to it,
`wakeci.db.before-<version>-<UTC time>` (mode 0600), with Bolt's
`tx.WriteTo` before anything writes, and keeps the newest 3. The version is
`dbVersion` in the global bucket. A new database isn't copied.

## Design decisions

- **A restart waits for builds instead of resuming them.** A task's output
  goes through a pipe into wakeci and a build's progress lives in a goroutine,
  so a running build can't outlive the process. Stopping waits for running
  builds (`draintimeout`), then aborts the rest the normal way so
  `on_aborted`/`finally` run; it never reruns them, since jobs such as deploys
  may not be safe to repeat. Queued builds are what survives: they are saved
  in the `queue` bucket by `BroadcastUpdate` (one transaction with the history
  entry) and restored with their job snapshot, params and API key flag. Their
  `on_pending` tasks don't run again. Builds, scheduled ones too (cron starts
  there), are started only after the port is bound, so a failed listen can't
  cut them short. A build is saved before it leaves the queue
  (`BroadcastUpdate` before `Cleanup`), since a stop closes the DB once the
  queue is empty, and `Build.updateMu` saves a build's updates in the order
  they were taken, so an older pending one can't bring its queue record back.
- **Sessions survive restarts.** Otherwise every update would sign everyone
  out. `SessionsBucket` keeps each session under `sessionKey`, the SHA-256 of
  its token, like API keys, so the db file (mode 0644) holds nothing that logs
  in; the in-memory map and `OnEnd`/`Hub.CloseSessions` use the same key.
  Every change takes `saveMu` until it is written, so a deletion (logout,
  revoke, expiry) can't be overtaken by a save and come back after a restart.
  A deletion that fails to save is kept in `unsaved` and retried with every
  later save and once more as wakeci stops. Since a restart no longer ends
  sessions, a password change ends every other password session
  (`DeletePasswordSessions`). It holds `passwordMu` for writing, which a login
  holds for reading until its session exists, so a login that checked the old
  password can't keep its session. The SSO login-state key is still generated
  at startup, so a restart only interrupts logins in progress.
- **No manual queue pause.** Stopping and updating already hold the queue, and
  a reboot stops wakeci the same way, so a pause button only added a second
  way to do it. The stop state isn't saved; it ends with the process. "Start
  now" doesn't work while stopping, since the build would delay or be cut by
  the restart.
- **The settings page checks for updates on every visit.** `HandleUpdateGet`
  caches a found release for `releaseCacheTTL` (10 minutes) to stay well
  inside GitHub's 60 unauthenticated requests per hour, and returns a failed
  check in `UpdateData.Error` with status 200, so a server without access to
  GitHub doesn't show an error on each visit. The E2E fixture answers
  `/api/update/` in the browser, so tests never reach GitHub. The update
  itself always asks GitHub again. The release notes link comes from GitHub's
  answer, so it is kept only if it starts with `releasePagePrefix`.
- **Self-update trusts the GitHub release, like `grm install`.** The sha256
  digest comes from the same GitHub API, so it catches broken downloads, not a
  compromised account. The update replaces the file wakeci was started from
  (symlinks resolved at startup), so the service user must be able to write
  its directory. It refuses a binary with file capabilities, which the new
  file wouldn't have (port 443 would fail); ambient capabilities survive the
  re-exec. There is no automatic rollback: `<binary>.previous` is kept for a
  manual one, and the update refuses if it can't create it. Any logged-in user
  may update, since they can already run commands as the service user; API
  keys can't.
- **A new version backs up the database before it changes it,** so an update
  can be undone with the data the old version left: stop wakeci, copy the
  backup over `wakeci.db`, put the old binary back (`<binary>.previous`, or
  `grm install jsnjack/wakeci==v<version>`) and start it. The copy is made in
  the restart an update needs anyway, so it adds no stop, only the time to
  copy; copying the file of a running wakeci isn't safe, since Bolt reuses
  freed pages. A failed copy is logged as an error and wakeci starts anyway,
  because a CI server that doesn't start is worse. The version is recorded
  either way, so a later start doesn't save the database this version already
  changed under its name.
