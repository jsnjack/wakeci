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
  job_files.go               Atomic job saves and persistent disabled-state preservation
  job_expand.go              Expands `include` and `block` task directives
  build.go                   Build/Task execution: runs tasks via go-cmd, writes task_N.log
  queue.go                   Queue of pending/running builds, concurrency limiting
  cleanup.go                 Periodic removal of old build workspaces/history
  eta.go                     Rolling average build duration, used for ETA display
  secrets.go                 injectSecrets: redacts/substitutes secret values in commands
  session.go                 Cookie-based in-memory sessions, each carrying a Principal
  auth.go                    Principal, Scope, RequireScope/RequireSession middleware
  auth_google.go             "Sign in with Google" (OIDC auth code + PKCE) handlers
  apikeys.go                 Scoped per-user API keys: storage, verification, handlers
  jobaccess.go               Limiting API keys to jobs: name patterns, RequireJob middleware
  preview.go                 Sandboxed, credential-bound report URLs; confined build files
  file_security.go           Owner-only workdir and database copies
  google_accounts.go         Automatic offboarding: re-checks Google users, revokes keys/sessions
  auth_limiter.go            Per-client failed-auth limiter and retry/reset response headers
  middleware.go               chi middleware: per-request logger, auth, CORS, security headers
  handlers_*.go              HTTP handlers per resource (auth, jobs, job, build, feed, settings, static, docs)
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
5. **Authentication** (`AuthMi`): an API key (`Authorization: Bearer wcik_...`
   or basic auth password starting `wcik_`), then the shared password via basic
   auth, then the `session` cookie. Whichever matches attaches a `Principal`
   to the request context; routes in `newRouter` then require a scope via
   `RequireScope`. Sessions (password or Google) hold every scope; API keys
   hold the scopes picked at creation, never more than their creator had.
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
| `hostname` | *(empty)* | Required for autocert when `port: 443` |
| `workdir` | `./wakeci` | BoltDB file, build workspaces, wakespace, certs |
| `jobdir` | `./` | Directory scanned/watched for `*.yaml` job files |
| `secretsfile` | *(empty)* | YAML map of secret values, injected via `injectSecrets` |
| `timezone` | *(empty)* | Default timezone for cron `interval` schedules |
| `google.client_id` / `client_secret` | *(empty)* | Enables "Sign in with Google" |
| `google.allowed_domains` | *(empty)* | Google Workspace domains that may sign in; required when enabled |
| `google.redirect_url` | *(derived)* | OAuth callback URL override |
| `google.disable_password` | `false` | Turns the shared password off, leaving Google as the only way in |

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
- **API keys are hashed with SHA-256, not bcrypt.** Their secret is 256
  random bits, so a slow hash adds per-request cost without making guessing
  harder. The token (`wcik_<16 hex id><64 hex secret>`) is shown to the user once.
- **API keys are managed only from browser sessions** (`RequireSession`), so
  a leaked key can't mint longer-lived or broader keys.
- **API keys can't hold `admin`** (`APIKeyScopes`). Admin can change the
  shared password, so an admin key would be full control in disguise.
- **Google ID tokens aren't signature-checked.** They are only ever read from
  Google's token endpoint over TLS in exchange for a single-use code with
  PKCE (OIDC Core §3.1.3.7). Domain allowlisting uses the `hd` claim, not the
  email suffix.
- **API keys are owned by `Principal.ID` (`google:<sub>`), not the email.**
  Google can reassign an email address to a new account; `sub` never changes.
- **Google sign-in is Workspace-only.** Accounts are matched on the `hd`
  claim, which personal Google accounts never have; there is deliberately no
  per-email allowlist for exceptions.
- **Offboarding needs no admin action.** Each Google login stores a refresh
  token (openid+email scopes only) in the `googleaccounts` bucket.
  `RunGoogleAccountChecker` redeems every token every 5 minutes; Google's
  `invalid_grant` (account suspended/deleted, access revoked) or a domain
  outside `allowed_domains` deletes the account, its API keys and sessions.
  Google's RISC push events don't cover Workspace users, hence polling.
- **Google access fails closed.** Keys and sessions of a Google user are
  refused unless Google confirmed the account within the last 30 minutes
  (`checkGoogleAccount`), so an outage of the checker can't keep a
  departed user's keys alive. An account is only re-confirmed with Google's
  current claims (ID token, else the userinfo endpoint), never stored ones.
- **The shared password works next to Google sign-in** unless
  `google.disable_password` is set (`passwordLoginEnabled`), so a deployment
  can move to Google gradually. Once set, login, basic auth, password
  sessions and keys created under `password:admin` all stop: a password
  can't be offboarded, and any Google user could set one to keep access
  after leaving.
- **Keys can be limited to jobs by name pattern** (`APIKey.Jobs`,
  `matchJobPattern`), where only `*` is special, so a pattern also covers
  jobs created later. Routes about one job or build check it with
  `RequireJob`; listings (feed, jobs list, websocket messages) filter
  instead. Builds are mapped to their job through the `history` bucket.
- **Build files are always served sandboxed** (`artifactCSP`, no
  `allow-same-origin`): artifacts are task output, and script in a
  same-origin page could drive wakeci in another window as the viewer. A
  sandboxed page sends no cookies, so HTML artifacts and directories are
  redirected to `/preview/<token>/build/...`, whose HMAC token (in-memory
  key, 15 minutes, one report directory or artifacts directory) lets the
  report's own CSS, scripts and images load. Tokens bind to a live session
  digest, the verified password fingerprint, or a live API key. Revocation
  ends the preview. `os.OpenRoot` confines storage to a single build and
  previews to the signed directory inside that build. App-origin script,
  style and worker subresource loads are refused. Token-authorized preview
  responses allow CORS from opaque origins so report modules and fonts load.
- **Offboarding is one transaction**: the account, its keys and its
  sessions go together, and `CreateAPIKey` re-checks the session inside the
  transaction that stores a key, so a request authenticated just before
  offboarding can't add one after it. Every login stores a new
  `googleAccount.LoginID`; the checker only acts if it's unchanged.
- **Websockets re-check their credential** (`credentialCheck`) every 30s and
  on `WSHub.Recheck()`, and close once it's revoked; a websocket otherwise
  streams build logs for as long as it stays open.
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
- Session errors omit cookie values; `Principal.LogValue` logs only identity metadata and never the password fingerprint.
- Never log `google.client_secret` or an API key token; log key IDs instead.
- App-owned job saves publish complete files atomically and share a mutex with registration cleanup. New jobs use mode 0600; updates preserve existing permissions. The `disabledjobs` bucket retains disabled names across file removal or invalidation; explicit enabling or a successful UI/API deletion clears that state. External file replacement preserves disabled state, while explicit delete/recreate starts fresh. Watcher remove/rename events re-check the current file before invalidating it.
- Preview redirects retain the report query string, and Vite proxies `/preview` alongside `/storage`.
- Startup removes stale registrations for non-job YAML; watcher updates also remove their database and cron entries. Malformed YAML retains registration for recovery, but routes and execution validate the current file before using it.
- New routes must go through `newRouter` with a `RequireScope` (see
  `TestRouteScopes`), or API keys get no access to them. A route about one
  job or build also needs `RequireJob`, and a listing must filter with
  `jobFilter` (see `TestJobLimitedKeyRoutes`), or job-limited keys reach
  every job.
- The config and secrets files are never jobs (`isProtectedFile`), even when
  they sit in `jobdir` as the README's `jobdir: ./` example has it:
  `RegisterJob`, `CleanupJobsBucket` and every `/api/job/{name}` route skip
  or 404 them. A new way to reach a job file must check it too.
- A valid API key must not reset the failed-auth limiter: it counts
  password guesses per client too.
- Every `MsgBroadcast` must set `job`: the hub drops messages a client's
  key can't see, and one without a job only reaches unlimited clients.
- `/storage/build/` paths are resolved like the file server will
  (`jobOfStoredFile` cleans them), so `1/../2/x` counts as build 2's.
- The Google state cookie is `SameSite=Lax` on purpose: the callback is a
  cross-site navigation from Google, which wouldn't carry a Strict cookie.
- Google only returns a refresh token on consent. The callback retries the
  login once with `prompt=consent` when it gets none and has none stored;
  without one, an account couldn't be offboarded.
- `wakeci.db` holds refresh tokens and is kept at mode 0600 (`dbFileMode`),
  on startup and by `--compactdb` for both the compacted db and the backup.
- Anything that revokes a credential (logout, key revocation, offboarding,
  password change) must call `WSHub.Recheck()`, and a new way to
  authenticate must be handled in `credentialCheck`.
- Changing the shared password revokes all password sessions and keys,
  including the caller's, their previews and websockets. Session fingerprints
  refuse stale password sessions even if issuance races with rotation.
  Google sessions remain active. Unknown identity providers fail closed.
- Logout is POST. `http.CrossOriginProtection` rejects cross-origin unsafe
  browser requests. Unsafe request bodies are buffered before authentication
  and limited to 4 MiB; servers bound header/body reads to 5s/30s. Passwords
  are body-only. The default `admin` password is refused while Google is on.
- All `/api` and `/auth` responses are `no-store`; storage/preview responses
  are `private, no-store` with `Vary: Cookie, Authorization`. API docs use
  pinned SRI assets in a sandbox; the public spec alone allows wildcard CORS.
- Trace logs are recreated exclusively with `O_NOFOLLOW` at mode 0600. Startup
  also restricts old database backups and compacted copies to 0600 and the
  workdir to 0700. Never log preview tokens, including a bare preview URL.
- Google error callbacks consume validated browser state. A malformed refresh
  ID token does not delete an account; it leaves verification stale. Explicit
  invalid grants, wrong subjects, disallowed domains and unverified email
  revoke access. Both successful and failed checks must respect LoginID.
- Caller parameters and `build.env` never expand secret references. Strip
  colours before log redaction. Only job-authored values can select secrets.
- Run dialogs submit displayed values and mark unchanged defaults with the `X-Wakeci-Default-Params` JSON array header. The backend treats a marked value as job-authored only when it matches the current definition loaded for that build. Stale defaults and caller overrides stay literal, including secret references; re-runs retain displayed values after job edits. UI requests also preserve explicitly empty overrides. API requests without the header retain their existing behavior.
- Write keys cannot be limited to jobs because editing a scheduled job can
  execute commands as the server. Verification refuses legacy restricted write
  keys too; creation and the UI must enforce the same boundary.
- Google login starts are limited per client address to 30 per minute and
  20 pending states. Clients behind one reverse proxy share these limits;
  forwarded client-IP headers are not trusted.
- `Build.Logger`, `Cleaner.Logger`, and the per-request `HL` context logger
  are all `*slog.Logger` instances scoped with `.With(...)`; they are
  unrelated to the per-task `task_N.log` files.
- `go.work` only includes `src/backend`; run Go tooling (`go vet`, `go test`,
  `golangci-lint`) from inside `src/backend`, not the repo root.
