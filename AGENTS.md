# AGENTS.md

> See [AGENTS.universal.md](./AGENTS.universal.md) and [AGENTS.go.md](./AGENTS.go.md) for universal conventions.
> Refresh: `make standards`

---

## Overview

wakeci is a self-hosted CI/automation server. Jobs are defined as YAML files
with an Ansible-inspired task syntax; the server watches the job directory,
schedules builds (manually, via API, or on a cron `interval`), streams task
output to a Vue frontend over WebSockets, and stores build history in an
embedded BoltDB database. It is aimed at small teams that want a
single-binary CI server without an external database or plugin ecosystem.

---

## Architecture

```
Makefile                     Builds frontend + backend into one binary, embeds assets
Wakefile.yaml                Server config (port, workdir, jobdir, secretsfile, timezone)
secrets.yaml                 Secret values injected into task commands at runtime

src/backend/                 Go server (package main, module "wakeci")
  main.go                    Entry point: flags, DB bootstrap, router, HTTP(S) listeners
  logger.go                  slog-based logger, --debug/--trace wiring, fanoutHandler
  config.go                  WakeConfig: loads Wakefile.yaml and secrets.yaml
  db.go                      BoltDB buckets, byte/int helpers, CompactDB (--compactdb)
  job.go                     Job/Task types, job file <-> DB sync, cron scheduling, fsnotify watcher
  job_expand.go              Expands `include` and `block` task directives
  build.go                   Build/Task execution: runs tasks via go-cmd, writes task_N.log
  queue.go                   Queue of pending/running builds, concurrency limiting
  cleanup.go                 Periodic removal of old build workspaces/history
  eta.go                     Rolling average build duration, used for ETA display
  secrets.go                 injectSecrets: redacts/substitutes secret values in commands
  session.go                 Cookie-based in-memory session storage, per-session TTL
  google_auth.go             Google Workspace sign-in (OpenID Connect code flow + PKCE)
  auth_limiter.go            Per-client failed-auth limiter and retry/reset response headers
  middleware.go               chi middleware: per-request logger, auth, CORS, security headers
  handlers_*.go              HTTP handlers per resource (auth incl. Google sign-in, jobs, job, build, feed, settings, static, docs)
  wsclient.go, wshub.go      WebSocket hub: pushes build/feed updates to connected browsers
  query_parser.go            Feed search query parser (`+must -exclude "phrase" key:value`)
  docs/, assets/             swag-generated OpenAPI spec and embedded frontend build (git-ignored)

src/frontend/                Vue 3 + Vite SPA
  src/views/                 Pages (feed, job editor, build view, settings, docs)
  src/components/            Shared Vue components
  src/store/                 Vuex store
  cypress/                   E2E tests (npm run test:dev / test:prod)
```

---

## Key Flows

1. **Startup** (`main.go`): parse flags → init logger → load `WakeConfig` → open
   BoltDB → bootstrap buckets/default password → start cron, job watcher,
   cleanup ticker, WebSocket hub → serve HTTP (or HTTP+HTTPS via autocert on
   port 443).
2. **Job registration**: job YAML files in `jobdir` are scanned at startup
   (`ScanAllJobs`) and watched via fsnotify (`InitJobWatcher`); each
   create/write/remove re-syncs the job's entry in the `jobs` BoltDB bucket
   and its cron schedule.
3. **Running a build**: `RunJob` creates a `Build`, expands `include`/`block`
   tasks, writes a workspace + wakespace + build config snapshot, and adds it
   to the `Queue`. The queue starts it when a concurrency slot is free.
4. **Task execution**: `Build.runTask` runs each task's `run` command via
   `go-cmd`, streaming stdout/stderr into `task_N.log` (in the build's
   wakespace dir) through `ProcessLogEntry`, which also broadcasts lines to
   subscribed WebSocket clients. This log path is independent of the
   application's own diagnostic logging (see Gotchas).
5. **Login**: the password form posts to `/auth/login`; Google sign-in is a
   full-page redirect `/auth/google/login` → Google → `/auth/google/callback`,
   which checks the state cookie, exchanges the code (PKCE + client secret),
   checks the ID token claims including `hd` against `google.domain`, and
   creates the same kind of session cookie. API clients use Basic auth with
   the password; `AuthMi` is unchanged.
6. **Live UI updates**: `Build.BroadcastUpdate` persists build state to the
   `history` bucket and pushes it over `WSHub` to any client subscribed to
   that build/feed topic.

---

## Build & Run

```bash
make build      # frontend (vite) + backend (swag docs + go build), outputs bin/wakeci
make runf       # frontend dev server (npm run serve)
make runb       # backend, rebuilt and restarted on *.go change (entr)
make test       # backend unit tests (alias for test_go)
make check      # fmt, vet, build, test, lint, isolated production E2E tests
```

Smoke test: `./bin/wakeci --config Wakefile.yaml` then open
`http://localhost:8081` (default password `admin`).

---

## Configuration

`Wakefile.yaml` (path via `--config`/`-c`, default `Wakefile.yaml`):

| Field | Default | Purpose |
|---|---|---|
| `port` | `8081` | Listen port; `443` enables autocert + port-80 redirect |
| `hostname` | *(empty)* | Required for autocert when `port: 443`; also sets CORS/HSTS and the default `google.redirecturl` |
| `workdir` | `./wakeci` | BoltDB file, build workspaces, wakespace, certs |
| `jobdir` | `./` | Directory scanned/watched for `*.yaml` job files; keep `Wakefile.yaml` and `secretsfile` outside it, since any `*.yaml` there can be opened as a job |
| `secretsfile` | *(empty)* | YAML map of secret values, injected via `injectSecrets` |
| `timezone` | *(empty)* | Default timezone for cron `interval` schedules |
| `google.clientid` | *(empty)* | Google OAuth client ID; setting it enables "Sign in with Google" |
| `google.clientsecret` | *(empty)* | Google OAuth client secret; required with `clientid` |
| `google.domain` | *(empty)* | Google Workspace domain allowed to log in (`hd` claim); required with `clientid` |
| `google.redirecturl` | `https://<hostname>/auth/google/callback` | Must match a redirect URI registered on the OAuth client; must be http(s) with path `/auth/google/callback`; required if `hostname` is empty |

Logging flags (all commands): `--debug`/`-d` (verbose, stderr),
`--trace` (everything, `/tmp/wakeci.log`, truncated each run). See
AGENTS.universal.md for the general contract.

---

## Design Decisions

- **Single binary, embedded frontend.** The Vue build output is embedded via
  `//go:embed assets/*` so deployment is one file; `make build` always
  rebuilds the frontend first.
- **BoltDB, not a client-server database.** Keeps the server dependency-free;
  `CompactDB`/`--compactdb` exists because Bolt doesn't reclaim space
  automatically.
- **Two independent logging paths.** The app's own operational logging
  (`L`, `--debug`/`--trace`) is separate from per-build task output
  (`task_N.log` via `ProcessLogEntry`), which is a product feature shown to
  users, not a diagnostic stream — never route one through the other.
- **Per-request/per-build loggers carry context via `slog.Logger.With`**
  (request ID + host, or build ID) rather than a shared global, so log lines
  from concurrent builds/requests stay attributable.
- **stdlib `flag`, not cobra.** wakeci is a single-command server with a
  handful of flags, not a multi-subcommand CLI; cobra would add a dependency
  and change flag syntax for no functional benefit.
- **Google sign-in sits next to the password, not instead of it.** It only
  creates a session; there are no per-user accounts or permissions. The ID
  token is not signature-checked because it comes straight from Google's
  token endpoint over TLS in exchange for the client secret (OpenID Connect
  Core 3.1.3.7), which avoids a JWT/JWKS dependency. Google sessions last
  12 hours (password sessions 5 days) because wakeci never re-checks the
  Google account: the TTL bounds how long a suspended account's session
  stays valid. It is not a hard revocation, since any logged-in user can
  change the password or schedule jobs. Which Workspace users may sign in is
  controlled in the Google Admin console, not in wakeci.
- **fsnotify watcher and the port-80 redirect listener never crash the
  server.** Both run in the background after the server is already serving
  traffic; setup or runtime failures there are logged and the feature
  degrades rather than taking down active builds.

---

## Gotchas

- `config.secrets` (loaded from `secretsfile`) must never be logged as part
  of the whole `WakeConfig` struct — log individual fields instead (see
  `config.go`).
- Never log the raw password from `HandleLogIn`/basic auth — only the error.
- Never log `google.clientsecret`; `CreateWakeConfig` logs the other
  `google.*` fields individually.
- The Google login-state cookie is `SameSite=Lax` (the callback is a
  cross-site top-level navigation from Google); the session cookie stays
  `Strict`. Don't "align" them.
- `Build.Logger`, `Cleaner.Logger`, and the per-request `HL` context logger
  are all `*slog.Logger` instances scoped with `.With(...)`; they are
  unrelated to the per-task `task_N.log` files.
- `go.work` only includes `src/backend`; run Go tooling (`go vet`, `go test`,
  `golangci-lint`) from inside `src/backend`, not the repo root.
