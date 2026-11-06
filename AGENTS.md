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
Wakefile.yaml                Server config (port, workdir, jobdir, secretsfile, timezone, sso)
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
  sso.go                     SAML single sign-on (e.g. Google Workspace): IdP metadata, response checks, login state
  apikeys.go                 API keys: storage, management handlers, the key router with its scope and job checks
  auth_limiter.go            Per-client failed-auth limiter and retry/reset response headers
  audit.go                   Audit log: who did what, AuditMi and the table of audited routes
  middleware.go               chi middleware: per-request logger, auth, CORS, security headers
  handlers_*.go              HTTP handlers per resource (auth incl. SSO, jobs, job, build, feed, settings, static, docs)
  wsclient.go, wshub.go      WebSocket hub: pushes build/feed updates to connected browsers
  query_parser.go            Feed search query parser (`+must -exclude "phrase" key:value`)
  docs/, assets/             swag-generated OpenAPI spec and embedded frontend build (git-ignored)

src/frontend/                Vue 3 + Vite SPA
  src/views/                 Pages (feed, job editor, build view, settings, docs)
  src/components/            Shared Vue components
  src/store/                 Vuex store
  e2e/                       Playwright E2E tests and isolated backend fixtures
  playwright.config.js       Parallel workers, Chrome, failure traces/screenshots
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
   application's own diagnostic logging (see Gotchas). An abort (the API or
   the job `timeout`) goes through `Queue.Abort` to `Build.requestAbort`,
   which closes the channel of the build's current `abortScope`; `runTask`,
   the condition wait, `runOnStatusTasks` and `Start` all check it.
5. **Login**: the password form posts to `/auth/login`. Single sign-on is a
   SAML 2.0 SP-initiated flow: `/auth/sso/login` redirects to the identity
   provider with an AuthnRequest and sets a signed login-state cookie holding
   the request ID; the identity provider posts the SAMLResponse to
   `/auth/sso/acs`, which checks the signature against the metadata
   certificate, issuer, audience, recipient, validity window and
   `InResponseTo`, then requires one of `sso.groups` in the `groups`
   attribute and creates the same kind of session cookie. API clients use
   an API key or Basic auth with the password; `AuthMi` is unchanged.
6. **API keys**: `routeAPIKeys` sends every request with
   `Authorization: Bearer wk_...` to a separate router (`newAPIKeyRouter`)
   that only has the routes keys may use. `APIKeyAuthMi` checks the token
   against the hash in the `apikeys` bucket and its expiry, then each route's
   `requireAPIKey(scope, jobResolver)` checks the scope and the job: from
   `{name}`, or from the build's history entry for `/build/{id}` and
   `/storage/build/{id}/...`. Anything not registered there gets 403. The
   feed and job list filter themselves with `apiKeyFromContext`.
7. **Live UI updates**: `Build.BroadcastUpdate` persists build state to the
   `history` bucket and pushes it over `WSHub` to any client subscribed to
   that build/feed topic.
8. **Audit log**: `LogMi` gives each request a `requestInfo` (log ID, IP,
   `X-Forwarded-For`); `AuthMi` and `APIKeyAuthMi` record who made it with
   `setRequestUser` once the credentials check out, and the request line
   names them. `AuditMi`, registered before the auth middleware on both
   routers, writes one `msg=audit` line for every request that can change
   something, with the event from `auditedRoutes`, what handlers added with
   `addAuditAttrs`, and the status. Logins, logouts and scheduled builds
   call `audit` / `auditScheduled` themselves.

---

## Build & Run

```bash
make build      # frontend (vite) + backend (swag docs + go build), outputs bin/wakeci
make runf       # frontend dev server (npm run serve)
make runb       # backend, rebuilt and restarted on *.go change (entr)
make test       # backend unit tests (alias for test_go)
make check      # fmt, vet, build, test, lint, isolated parallel production E2E tests
make testprod   # build once, then run Playwright E2E tests in parallel
make testdev    # build once, then run the isolated E2E suite with visible browsers
make testreport # open the last E2E HTML report from the repository root
make release    # test + testprod, then publish bin/wakeci as a GitHub release
```

Smoke test: `./bin/wakeci --config Wakefile.yaml` then open
`http://localhost:8081` (default password `admin`).

E2E tests use installed Google Chrome by default. Each Playwright worker starts
its own backend on an available port with a temporary database, job directory,
and test secrets. Tests use separate browser contexts and unique files/job names;
the fixture waits for the file watcher to register new jobs, removes jobs, and
aborts unfinished builds after each test. Browser sessions for login/settings
tests are separate from the worker's API session.
Use `make testprod E2E_ARGS="--workers=4"` to set the worker count, or
`E2E_ARGS="e2e/feed.spec.js"` to run selected specs. Direct `npm test` runs use
the existing `bin/wakeci`; rebuild first after application changes. The default
worker count is half the available CPUs, capped at ten. Failure artifacts go
to `src/frontend/test-results/`; the HTML report goes to
`src/frontend/playwright-report/`. Worker logs use `--debug` and private files,
because `--trace` would make concurrent backends overwrite `/tmp/wakeci.log`.

---

## Configuration

`Wakefile.yaml` (path via `--config`/`-c`, default `Wakefile.yaml`):

| Field | Default | Purpose |
|---|---|---|
| `port` | `8081` | Listen port; `443` enables autocert + port-80 redirect |
| `hostname` | *(empty)* | Required for autocert when `port: 443`; also sets CORS/HSTS and the default `sso.acsurl` |
| `workdir` | `./wakeci` | BoltDB file, build workspaces, wakespace, certs |
| `jobdir` | `./` | Directory scanned/watched for `*.yaml` job files; keep `Wakefile.yaml` and `secretsfile` outside it, since any `*.yaml` there can be opened as a job |
| `secretsfile` | *(empty)* | YAML map of secret values, injected via `injectSecrets` |
| `timezone` | *(empty)* | Default timezone for cron `interval` schedules |
| `sso.idpmetadata` | *(empty)* | Path to the identity provider's SAML metadata; setting it enables "Sign in with SSO" |
| `sso.groups` | *(empty)* | Comma-separated group names allowed to log in, matched case-insensitively against the `groups` attribute; required with `idpmetadata` |
| `sso.acsurl` | `https://<hostname>/auth/sso/acs` | Where the identity provider posts the response; must be https (http only on localhost) with path `/auth/sso/acs`; required if `hostname` is empty |
| `sso.entityid` | scheme and host of `sso.acsurl` | SP entity ID; must match the Entity ID registered at the identity provider (checked as the assertion audience) |

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
- **The audit log is a third path** (`auditLogger`, always on, stderr,
  text, `msg=audit`): a record for auditors of who did what, from where,
  not diagnostics, so `--debug` doesn't turn it off. One line per sign-in
  and per change; reads are left to the request line, which names the
  user the same way on every route behind `AuthMi` (the SPA's pages are
  public and name no one), to keep the audit log small enough to read. `AuditMi`
  wraps the routers instead of each handler calling it, so every error
  branch and every refusal (by `AuthMi`, a key's scope, a missing route) is
  recorded with its status, and it finds the route with its own `auditMux`
  because the auth middleware refuses requests before chi has matched them.
  `ip` is the connection's address; `X-Forwarded-For` is logged next to it
  as sent, never trusted, since anyone can set it and wakeci doesn't know
  if a proxy is in front. A long header keeps its start and its end, where
  a proxy appends. `audit` cuts every string value at 1024 bytes, so a
  client can't write huge lines (e.g. with a long URL). Lists of names go
  through `auditListAttrs`: JSON, since names can hold commas, kept valid
  within the limit by leaving names out and counting them.
- **Per-request/per-build loggers carry context via `slog.Logger.With`**
  (request ID + host, or build ID) rather than a shared global, so log lines
  from concurrent builds/requests stay attributable.
- **stdlib `flag`, not cobra.** wakeci is a single-command server with a
  handful of flags, not a multi-subcommand CLI; cobra would add a dependency
  and change flag syntax for no functional benefit.
- **Single sign-on sits next to the password, not instead of it.** It only
  creates a session; there are no per-user accounts or permissions.
- **SAML, not OpenID Connect.** Who may log in is decided by group, and
  Google Workspace only sends group membership to SAML apps; Google's OIDC ID
  tokens carry no groups. Signature checking is left to
  `russellhaering/gosaml2` + `goxmldsig`; `VerifyResponse` adds what the
  library only reports as warnings or leaves to the caller (validity window,
  a required and matching audience, a single assertion, `InResponseTo`). `InResponseTo` is read from
  the signed assertion, not the Response element, which Google may leave
  unsigned. Unsolicited (IdP-initiated) responses are never accepted; a
  correctly signed one restarts an SP-initiated login instead.
- **A response is accepted once, for the browser that asked for it.** The
  login-state cookie is HMAC-signed with a key generated at startup, so a
  captured response can't be paired with a forged cookie, and
  `usedRequests` remembers answered request IDs until their cookie expires,
  so a captured response and cookie can't be replayed together. Only
  correctly signed responses that match a valid cookie reach that map, so it
  can't be flooded. A restart invalidates logins in progress, which only
  means a retry.
- **API keys have their own router, not a branch in `AuthMi`.** A route is
  closed to keys until it is registered in `newAPIKeyRouter` with a scope and
  a job resolver, so a new route can't be opened to keys by accident. Keys
  never reach the password or session code, and never open settings, key
  management, `/auth` or `/ws` (no long-lived connection to re-check after a
  revoke).
- **API key scopes**: `read` (lists, job files, builds, logs, artifacts),
  `run` (run, abort, start, flush), `write` (create, edit, delete, enable).
  They don't include each other. `write` requires all jobs (`AllJobs`),
  checked at creation and again in `HasScope`, because a job runs commands on
  the server and can reach every other job and secret.
- **The admin password manages every key and alone changes the password;
  an SSO user manages only their own keys.** An SSO user who could set the
  password would log in with it and be the admin, so `HandleSettingsPost`
  refuses a password change without the admin password (403, nothing
  saved) and the settings page hides the field. Revoking a person's keys
  also ends their sessions, first and under `apiKeyOwnersMu`, which key
  creation takes too, so a session they still have can't make a new key.
  Ended sessions (revoke, logout, expiry) are passed to
  `SessionStorage.OnEnd`, which closes the WebSockets opened with them
  (`Hub.CloseSessions`), since `/ws` only checks auth at the upgrade. A
  request can pass `AuthMi` and register only after its session ended, so
  the hub checks the session again when it registers a client. The
  admin's person list (`APIKeysData.People`) includes everyone signed in
  with SSO, so someone without keys can be signed out too. A revoke also
  records a cutoff per person (`SessionStorage.revoked`): an SSO login whose
  signed assertion was issued before it, plus a minute for clock skew, is
  refused by `NewSSO`, under the same lock as `DeleteUser`, so a login
  obtained before the offboarding can't recreate a session. A fresh login
  still works unless Google stopped the account.
  This is not a role system: SSO users can still edit jobs, which runs
  commands on the server, so they are trusted; roles are future work.
  Sessions carry the SSO email (`session.user`, empty for the password) and
  each key records it in `CreatedBy`. `requestUser` reads it, Basic auth
  first like `AuthMi`. The admin password (password login or Basic auth)
  lists and revokes every key and can revoke all keys of a person
  (`DELETE /api/keys/?createdBy=`); an SSO user lists and revokes only keys
  with their email, and someone else's key is a 404 to them. Offboarding is
  this manual step: wakeci can't see a Google account go (custom SAML apps
  get no provisioning), and keys used by agents mean their owner may never
  sign in again, so expiring keys on inactivity would break live automation.
- **API key secrets are hashed with SHA-256, not bcrypt.** A secret is 256
  random bits, so there is nothing to guess; bcrypt would slow down every
  API call. Tokens are `wk_<id>_<secret>`: the ID is public (list, logs,
  revoke), and only the hash of the secret is stored.
- **API keys can't send run parameters that reference a secret or hold a
  quote, line break or NUL** (`rejectUnsafeParams`). Run parameters go
  through `injectSecrets`, so a run key could otherwise send any secret
  wherever the job sends that parameter. A line break, or a quote on one
  line, would let it add variables such as `BASH_ENV` to `build.env` through
  the documented `echo "NAME=$VALUE"`: godotenv starts a new variable after
  a quoted value (`TARGET="a" BASH_ENV=...`). Without quotes it reads the
  rest of the line as one value, checked for godotenv v1.5.1. In
  builds a key starts (`Build.startedWithAPIKey`, passed to `CreateBuild`
  because it starts the `on_pending` tasks), `build.env` values skip
  `injectSecrets`, because godotenv expands variables and a job can rewrite a
  param, so a reference could be put together after the check. Parameters
  from the UI, cron and the password are unchanged. This can't stop a job
  that sends a secret to a destination taken from a parameter; such a job
  trusts whoever can run it.
- **Build artifacts run sandboxed on wakeci's own origin**
  (`storageCSP` in `StorageSecurityMi`). An HTML report is built by a job and
  opened by a logged-in user, so it must not act as that user. It can't send
  requests (`connect-src`, `form-action`), and it can't reach another page on
  the origin to send them from: no frames or objects, no popups (the
  sandbox has no `allow-popups`), no service workers. It keeps its origin
  (`allow-same-origin`) so its own CSS, scripts and images still get the
  SameSite session cookie; an opaque origin breaks multi-file reports. PDFs
  keep the policy without the sandbox, since Chrome won't show a sandboxed
  PDF and a PDF can't run scripts on the origin. A separate artifact origin
  would be stronger, but needs a second hostname and its own auth.
- **SSO sessions last 12 hours** (password sessions 5 days) because wakeci
  never re-checks the account or its groups: the TTL bounds how long a
  suspended account, or one removed from the allowed groups, keeps access.
  It is not a hard revocation, since any logged-in user can change the
  password or schedule jobs.
- **An abort is a closed channel, not a send.** The build spends time
  outside a running command (setup, `when`/`if` checks, between tasks,
  hooks), so a signal sent to whoever listens at that moment was lost, and
  a blocking send froze the queue. A closed channel stays readable and
  closing never blocks. Each group of tasks gets a fresh scope
  (`nextAbortScope`) only if the previous one was aborted, so the abort that
  stopped the main tasks doesn't also stop `on_aborted`, `finally` always
  runs, and an abort that arrives between two groups stops the next one.
  A build aborted while queued moves to `Queue.aborting`: it can't be
  started and holds no concurrency slot, but `Abort` still reaches its
  `on_aborted` and `finally` tasks.
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
- The SSO login-state cookie is `SameSite=None; Secure` because the
  identity provider POSTs the response cross-site and browsers leave `Lax`
  cookies off cross-site POSTs; the session cookie stays `Strict`. Don't
  "align" them. SSO therefore needs HTTPS (browsers accept `Secure` cookies
  on `http://localhost`).
- `sso.groups` must match what the identity provider sends: Google sends
  group names, not emails. A rejected login logs the received groups.
- `Build.Logger`, `Cleaner.Logger`, and the per-request `HL` context logger
  are all `*slog.Logger` instances scoped with `.With(...)`; they are
  unrelated to the per-task `task_N.log` files.
- When adding an API route, decide whether API keys may use it. If so,
  register it in `newAPIKeyRouter` too, with its scope and job resolver, and
  make list handlers filter with `apiKeyFromContext`.
- A new route that changes something needs an entry in `auditedRoutes`
  (`TestAuditedRoutes` fails without one). Audit attrs carry names and IDs,
  never passwords, tokens, secrets or run parameter values; errors that may
  be logged must not contain them either (session errors don't name the
  token). What a route acts on comes from the URL in `auditedRoutes`
  (`attrs`), so a request refused before the handler still names it; a
  pattern ending in `/` also matches without it, as chi serves a mounted
  router's root.
- A handler that changes several things checks them all before saving any,
  then saves them in one transaction (`HandleSettingsPost`), so a request
  that fails changed nothing and its audit outcome is true.
- Login handlers start with `withRequestInfo`, so `setRequestUser` and
  `audit` share one `requestInfo` even when called without `LogMi` (tests).
- `jobFromStorage` must read the path from `wakespacePath` and clean it like
  `http.FileServer` does, so the build it checks is the build that is served
  (`/storage/build/1/../2/x` is build 2).
- API key failures count on `apiKeyAttemptLimiter`, never on
  `globalAuthAttemptLimiter`, so a valid key can't clear failed password
  attempts. Only failures are limited: a valid key always passes, so bad
  keys from the same address (or proxy) can't lock it out. On a bad key,
  `reject` runs before `recordFailure` because only `reject` clears an
  expired block; without it bad keys get 429 forever. A refused build
  doesn't name its job. Never log a token; the key ID is fine.
- Don't loosen `storageCSP`: `allow-popups`, a missing `frame-src`,
  `object-src` or `worker-src`, or `frame-ancestors 'self'` each let an
  artifact reach a page that can call the API (checked in Chrome: an
  `<object>` and a popup created API keys, and a service worker registered).
  Dropping `allow-same-origin` breaks reports' own files instead. The app
  must keep opening artifacts with `rel="noopener"` and never frame them.
- `HandleFeedView` scans the history in a read transaction, so a slow or
  filtered scan never holds Bolt's single write lock; builds the queue no
  longer knows are marked aborted afterwards by `markStaleBuildsAborted`,
  which checks them again.
- `requestUser` takes any Basic auth as the admin password because `AuthMi`
  has checked it first and never falls back to the cookie. Test `/api`
  through `apiRoutes` (as `apiRouter` in the tests does), so `AuthMi` is in
  the path.
- `go.work` only includes `src/backend`; run Go tooling (`go vet`, `go test`,
  `golangci-lint`) from inside `src/backend`, not the repo root.
