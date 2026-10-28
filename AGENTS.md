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
  auth_limiter.go            Per-client failed-auth limiter and retry/reset response headers
  middleware.go               chi middleware: per-request logger, auth, CORS, security headers
  handlers_*.go              HTTP handlers per resource (auth incl. SSO, jobs, job, build, feed, settings, static, docs)
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
5. **Login**: the password form posts to `/auth/login`. Single sign-on is a
   SAML 2.0 SP-initiated flow: `/auth/sso/login` redirects to the identity
   provider with an AuthnRequest and sets a signed login-state cookie holding
   the request ID; the identity provider posts the SAMLResponse to
   `/auth/sso/acs`, which checks the signature against the metadata
   certificate, issuer, audience, recipient, validity window and
   `InResponseTo`, then requires one of `sso.groups` in the `groups`
   attribute and creates the same kind of session cookie. API clients use
   Basic auth with the password; `AuthMi` is unchanged.
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
- **SSO sessions last 12 hours** (password sessions 5 days) because wakeci
  never re-checks the account or its groups: the TTL bounds how long a
  suspended account, or one removed from the allowed groups, keeps access.
  It is not a hard revocation, since any logged-in user can change the
  password or schedule jobs.
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
- `go.work` only includes `src/backend`; run Go tooling (`go vet`, `go test`,
  `golangci-lint`) from inside `src/backend`, not the repo root.
