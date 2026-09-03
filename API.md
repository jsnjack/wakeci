# wakeci API

wakeci exposes a small HTTP API used by the web frontend and suitable for
scripting. It consists of:

- a **REST API** under `/api` (and the authentication helpers under `/auth`);
- a **WebSocket API** at `/ws` for real-time build and log updates;
- a **storage endpoint** under `/storage/build/` that serves build artifacts and
  task logs.

A reference generated from the [swag](https://github.com/swaggo/swag)
annotations in the source is served by a running instance at
[`/docs/api/`](http://localhost:8081/docs/api/), with the raw OpenAPI/Swagger
spec at `/docs/swagger.json`. It covers the `/api` endpoints only — never
`/ws`, `/auth` or `/storage` — and the annotations lag behind the router:
`POST /api/build/{id}/start` is missing from it entirely, and several paths are
spelled with a trailing slash the router rejects. **This document is the
authoritative reference.**

> All examples below assume the server runs on `http://localhost:8081` (the
> default port). Adjust the host and scheme to your deployment: with `port: 443`
> wakeci serves HTTPS directly, obtaining a certificate through ACME/autocert
> and running a redirect listener on `:80`.

## Authentication

Every endpoint under `/ws`, `/api`, `/storage` and `/auth/_isLoggedIn` is
protected. Everything else is served anonymously: `POST /auth/login`,
`GET /auth/logout`, the interactive documentation under `/docs` and the frontend
assets.

A request to a protected endpoint is authenticated if it provides **either** of
the following:

1. **HTTP Basic authentication** — send the password as the password component;
   the username is ignored. This is the recommended method for scripts:

   ```bash
   curl -u :admin http://localhost:8081/api/feed
   ```

2. **Session cookie** — obtain a `session` cookie from `POST /auth/login` and
   send it back on subsequent requests. This is what the web frontend uses. The
   cookie is `HttpOnly`, `SameSite=Strict`, `Path=/` and valid for 120 hours
   (5 days). It additionally carries `Secure` whenever the request looks like
   HTTPS — the server is configured with `port: 443`, the connection is TLS, or
   the first value of `X-Forwarded-Proto` is `https` (case-insensitive). A
   TLS-terminating reverse proxy must forward that header, otherwise the cookie
   is issued without `Secure`.

If neither is present or valid, the server replies with `403 Forbidden` and the
body `Forbidden`. Two other failures are possible: `429 Too Many Requests`
(body `Too many authentication attempts`) when the client is blocked by the
rate limiter described below, and `500 Internal Server Error` (body
`Another triumph`) if the stored password hash cannot be read from the database.

> The default password is `admin`. Change it immediately via
> `POST /api/settings` or the Settings page in the UI.
>
> There is **no** localhost/internal bypass: requests from any origin must
> authenticate.

### Failed-authentication rate limiting

Password checks — `POST /auth/login` and HTTP Basic authentication on any
protected endpoint — share a single per-client failure counter:

- **100** failed password checks, each less than **5 minutes** after the
  previous one, block the client for **5 minutes**. The counter resets only
  after 5 minutes pass with no failure at all, and the 100th failure is already
  answered with the block rather than with `403`.
- The counter is keyed on the client IP (the host part of the TCP peer
  address). `X-Forwarded-For` is **not** consulted, so behind a reverse proxy
  every client shares one counter.
- Any successful password check clears the counter.
- Only password checks count. A missing, unknown or expired `session` cookie is
  answered with `403` and is never rate limited.

While blocked, every `POST /auth/login` and every request carrying an
`Authorization: Basic` header — **including one with the correct password** — is
rejected before the password is even checked, so a blocked client cannot log in
to clear its own counter:

`429 Too Many Requests`, body `Too many authentication attempts`, with

- `Retry-After` — seconds until the block expires (rounded up, minimum `1`);
- `RateLimit-Reset` — the same value;
- `X-RateLimit-Limit` — `100`;
- `X-RateLimit-Remaining` — `0`;
- `X-RateLimit-Reset` — Unix timestamp at which the block expires.

> The block does **not** cut off a client that authenticates by cookie: the
> limiter is consulted only on the Basic-auth branch, which is tried first and
> only when an `Authorization` header is present. A request from a blocked IP
> carrying a valid `session` cookie and no `Authorization` header is served
> normally.

## Conventions

- Request bodies for mutating endpoints are `application/x-www-form-urlencoded`
  (form data). Every input except the build parameters of
  `POST /api/job/{name}/run` is read with Go's `FormValue`, which merges the
  query string with the parsed body and accepts a `multipart/form-data` body as
  well, so any documented form field may equally be sent as a query parameter.
  `POST /api/job/{name}/run` is the exception: it parses the request itself and
  reads only the URL-encoded body and the query string, so build parameters sent
  as `multipart/form-data` are silently ignored.
- Paths are matched **exactly** — nothing adds or strips a trailing slash.
  `/api/feed/`, `/api/settings/` and `/api/job/{name}/` return `404 Not Found`,
  while `/docs/api/` **requires** its trailing slash. Only `/api/jobs` and
  `/api/jobs/` are interchangeable, because `/jobs` is a mount point.
- Only the method documented for a path is routed; anything else returns
  `405 Method Not Allowed` with an empty body and an `Allow` header. No
  `OPTIONS` route is registered, so a browser CORS preflight is answered with
  `405`.
- Successful responses are `application/json` unless stated otherwise; error
  bodies are plain text (`text/plain; charset=utf-8`) and usually carry the raw
  Go error message.
- Timestamps (`startedAt`) are ISO-8601 with fixed microsecond precision (extra
  digits are truncated, not rounded) in the server's local timezone, e.g.
  `2026-06-13T10:30:45.123456+02:00`. A build that has not started running yet,
  and every task that has not started yet, report the Go zero time
  `0001-01-01T00:00:00.000000Z` — check for that value rather than for `null` or
  a missing field.
- Durations (`duration`) are integers expressed in **nanoseconds**
  (Go `time.Duration`). A build's `duration` is `0` while it is `pending` or
  `running`; it is measured from the `running` transition until all of the
  build's terminal work is done. The `on_running`, `main`, terminal `on_*` and
  `finally` tasks and any artifact collection are inside that span; the time
  spent queued and the `on_pending` tasks are not — those finish before
  `startedAt` is stamped. Because the status is set before the terminal tasks
  start and the duration only after they return, a build whose job declares
  `on_failed`/`on_finished`/`on_aborted` or `finally` tasks first reports its
  **final status together with `duration: 0`**, and the real value appears only
  in the update that follows the last of them. Those intermediate states are
  stored like any other update, so `GET /api/feed` and `GET /api/build/{id}` can
  return that pair too; a build with no terminal tasks never exposes it. A
  task's `duration` is `0` until that task completes.
- A final status is not a promise of a real duration. A build aborted through
  `POST /api/build/{id}/abort` while it was still queued never started and ends
  with the saturated `duration: 9223372036854775807` beside the zero
  `startedAt`; a build left `pending` or `running` by a server restart keeps
  `duration: 0` permanently once `GET /api/feed` rewrites its status to
  `aborted`. Detect "never ran" from the zero `startedAt`, which both cases
  share — not from the saturated value.
- `eta` is an estimate in **nanoseconds** (the same unit as `duration`, despite
  the name): the average duration of the job's last 5 builds that ended
  `finished`. It is `0` until the job has 5 such builds on record, aborted and
  failed builds never contribute, and it is computed once when the build is
  created — it is never refreshed while the build runs.
- A **build status** is one of: `pending`, `running`, `finished`, `failed`,
  `aborted`, `timed out`.
- A **task status** is one of those plus `skipped` (the task's `when` or `if`
  condition evaluated to false; a skipped task does not stop the build).
  `skipped` is never a build status. `aborted` and `timed out` are reported on
  the task that was executing when the signal arrived, and tasks that never run
  — the `on_failed` tasks of a successful build, say — keep their initial
  `pending` status.
- A task **kind** is `main` for regular tasks, or one of `pending`, `running`,
  `failed`, `aborted`, `finished`, `finally` — the tasks of the job file's
  `on_pending`, `on_running`, `on_failed`, `on_aborted`, `on_finished` and
  `finally` sections. There is no `on_timed_out` and no `on_skipped` section: a
  build that times out runs its `on_aborted` tasks.
- `job.tasks` and `status_update.tasks` are the same flat list in execution
  order — `pending`, `running`, `main`, `failed`, `aborted`, `finished`,
  `finally` — and a task's `id` is its index in that list, so the first `main`
  task is **not** necessarily `id: 0`. `on_pending` tasks always complete before
  the build starts running, `on_running` tasks before the first `main` task, and
  `finally` tasks run last.
- **`null` is not `[]`.** Collections backed by an unset Go slice serialise as
  the JSON literal `null`: `params`, `artifacts` and `build_artifacts` on a
  build, `defaultParams` in `GET /api/jobs/`, and the whole body of
  `GET /api/feed` and `GET /api/jobs/` when there is nothing to return. Only
  `tasks` is guaranteed to be an array. Clients must handle `null` as "empty"
  (`response.data || []`).

## Response headers

Every response carries:

- `referrer-policy: no-referrer`
- `x-content-type-options: nosniff`
- `content-security-policy: default-src 'self'; style-src 'self' 'unsafe-inline'; frame-ancestors 'self'`
- `strict-transport-security: max-age=15768000;includeSubdomains` — only when
  `hostname` is set in the configuration file
- `access-control-allow-origin: *`, or `https://<hostname>` when `hostname` is
  set
- `access-control-max-age: 86400`

No `Access-Control-Allow-Credentials`, `-Methods` or `-Headers` are sent and no
`OPTIONS` route exists, so cross-origin browser calls that need a preflight or
that rely on the `session` cookie will not work — use Basic authentication from
a server-side client instead.

`/storage/build/*` and `/docs/swagger.json` replace the app-wide CSP with the
storage policy described in the storage section, and `/docs/api/` sets a third
one of its own that additionally allows the Swagger UI bundle from
`https://unpkg.com`. On a successful WebSocket upgrade none of these headers are
sent at all, because the `101` response is written by the WebSocket library
itself.

---

## Authentication endpoints

### POST /auth/login

Verify the password and start a session. On success, a `session` cookie is set
in the response.

**Input** (form data)

- `password` — `string`, required.

**Responses**

- `204 No Content` — password correct, `session` cookie set (the client's
  failure counter is cleared).
- `403 Forbidden` — incorrect password (body: `Incorrect password`).
- `429 Too Many Requests` — the client is blocked by the failed-authentication
  rate limiter (body: `Too many authentication attempts`). Returned even when
  the password is correct, and returned instead of `403` for the failure that
  trips the limit.
- `500 Internal Server Error` — database or session-creation error.

```bash
curl -i -X POST http://localhost:8081/auth/login -d 'password=admin'
```

### GET /auth/logout

Invalidate the current session (if any) and clear the `session` cookie. The
clearing `Set-Cookie` repeats the attributes of the login cookie (`Path=/`,
`HttpOnly`, `SameSite=Strict`, and `Secure` under the same conditions) with
`Max-Age=-1` and an expiry at the Unix epoch. The endpoint itself is not
authenticated and always returns `204 No Content`.

### GET /auth/_isLoggedIn

Lightweight authentication probe. Returns `200 OK` with an empty body when the
request is authenticated and `403 Forbidden` with the body `Forbidden` when it
is not. A probe that carries an `Authorization: Basic` header from a
rate-limited client returns `429 Too Many Requests` with the body `Too many
authentication attempts` instead; a probe authenticating by `session` cookie is
never rate limited.

---

## Feed

### GET /api/feed

Return information about the latest builds (newest first), at most **15** per
page.

**Input** (query parameters)

- `offset` — `integer`, optional. Skip the *n* most recent builds (for
  pagination). Defaults to `0`. A value that is not a valid integer is rejected
  with `400`; a **negative** value is accepted and silently treated as `1`, not
  as `0`.
- `filter` — `string`, optional. Space-separated terms matched (case-sensitive,
  as plain substrings) against the Go-formatted string
  `id:<id> name:<name> status:<status> <params>`, where `<params>` is Go's
  rendering of the parameter list, e.g.

  ```
  id:42 name:deploy-service status:finished [map[VERSION:1.2.3]]
  ```

  A build without parameters ends in the literal `[]`, and keys inside one
  mapping are printed alphabetically. Match a parameter as `VERSION:1.2.3` —
  never as `VERSION=1.2.3` or `"VERSION": "1.2.3"`. Because matching is
  substring-based, `id:4` also matches build 42.

  Prefix a term with `+` to require it, `-` to exclude it; unprefixed terms are
  matched with OR logic. Wrap multi-word terms in single or double quotes, e.g.
  `"status:timed out"`.

> The `+`/`-` prefix must sit **outside** the quotes: `-"test build"` excludes
> the phrase `test build`, while `"-test"` searches for the literal text
> `-test`. Quotes are stripped only when a term starts and ends with the same
> quote character, and only one layer is removed — an unbalanced quote stays
> part of the term. Terms are separated by the **space character only**: tabs
> and newlines are ordinary characters inside a term, and a `filter` made
> entirely of whitespace is equivalent to sending no `filter` at all.

> `offset` means different things with and without a `filter`. Without one it is
> applied to build **IDs** — the scan starts at `<newest build id> - offset` —
> so it behaves as "skip *n* builds" only while the history is contiguous: an
> `offset` larger than the newest build id returns `null`, and one pointing into
> already pruned history (see `buildHistorySize`) returns a short page rather
> than 15 builds. With a `filter`, the scan always restarts at the newest build
> and `offset` counts only **matching** builds. Paginate either way with
> `offset += 15`, but never reuse an offset across a filtered and an unfiltered
> request.

**Response** `200 OK` — JSON array of build objects:

```json
[
  {
    "id": 42,
    "name": "deploy-service",
    "status": "finished",
    "tasks": [
      {
        "id": 0,
        "status": "finished",
        "startedAt": "2026-06-13T10:30:45.000000+02:00",
        "duration": 5000000000,
        "kind": "main"
      }
    ],
    "params": [{ "VERSION": "1.2.3" }],
    "artifacts": ["output/service.tar.gz"],
    "build_artifacts": [{ "filename": "output/service.tar.gz", "size": 102400 }],
    "startedAt": "2026-06-13T10:30:45.000000+02:00",
    "duration": 13000000000,
    "eta": 12000000000
  }
]
```

> When nothing is returned — empty build history, a `filter` with no hits, or an
> `offset` larger than the newest build id — the body is the JSON literal
> `null`, **not** an empty array.

> `artifacts` (array of strings) is **deprecated** in favour of
> `build_artifacts` (objects with `filename` and `size` in bytes). Both are
> filled together and always list the same files in the same order:
> `artifacts[i]` is `build_artifacts[i].filename`. Both are `null` until the
> build collects its artifacts, and artifacts are only collected for builds that
> end `finished` or `failed` — a build that ends `aborted` or `timed out`
> reports `null` even when the job declares `artifacts:` patterns.

> Pending or running builds that are in neither the running nor the queued list
> are rewritten to `aborted` **in the database** as they are scanned, so the
> correction is permanent and also visible from `GET /api/build/{id}`. Only the
> builds scanned for the current page are corrected, and the rewrite happens
> before the `filter` is applied — a stale build matches `status:aborted`, not
> `status:running`.

**Other responses**

- `400 Bad Request` — `offset` is not an integer. Body is plain text with the
  value quoted, e.g. `Invalid offset: "abc"`.
- `500 Internal Server Error` — database read/write error, or a failure to
  encode the payload. Body is the raw Go error message.

---

## Jobs

### GET /api/jobs/

Return the list of available jobs.

**Response** `200 OK`:

```json
[
  {
    "name": "deploy-service",
    "desc": "Deploy the service",
    "defaultParams": [{ "VERSION": "main" }],
    "interval": "@every 2h",
    "active": "true"
  }
]
```

`active` is the string `"true"` or `"false"`. `interval` is a cron expression
(or empty when the job is not scheduled). `defaultParams` is `null` when the job
declares no `params:`, and the whole response is `null` (not `[]`) when no job
is registered.

**Other responses**: `500 Internal Server Error` (database or unmarshalling
error, message in the body).

### POST /api/jobs/create

Create a new job from the default template. The job is stored as a YAML file
named after `name` in the configured jobs directory.

**Input** (form data)

- `name` — `string`, required. Name of the job (and of its file). It must be
  non-empty and must not contain `/` or `\`: the name is used directly as a file
  name inside the jobs directory.

**Responses**

- `200 OK` — job created (empty body).
- `400 Bad Request` — the name is empty or contains a path separator (body:
  `Invalid job name`), a job with this name already exists (body: `Job with this
  name already exists`), or the file could not be written.
- `500 Internal Server Error` — template or registration error.

---

## Job

All `/api/job/{name}` endpoints take the job name as a URL path parameter, and
it must be **percent-encoded**: the server decodes it once and then validates
it, so names containing reserved characters work (`/api/job/my%20job`), while an
encoded path separator does not.

A decoded name that contains `/` (as `%2F`) or `\` is rejected with
`400 Bad Request` and the plain-text body

```
invalid job name: a/b
```

before the endpoint does any work — this applies to every endpoint in this
section, in addition to the responses listed below. An empty name
(`/api/job//run`) is not routed at all and returns `404 Not Found`.

### GET /api/job/{name}

Return the raw content of the job file.

**Response** `200 OK`:

```json
{ "fileContent": "desc: Deploy the service\ntasks:\n  - name: Build\n    run: make build\n" }
```

`400 Bad Request` if the job name is invalid. `500 Internal Server Error` if the
file cannot be read — **including when no such job exists**; this route has no
`404`, and the body is the underlying error, e.g.
`open jobs/ghost.yaml: no such file or directory`.

### POST /api/job/{name}

Replace the content of the job file.

**Input** (form data)

- `fileContent` — `string`. The new YAML content. It is validated for YAML
  syntax and for a parsable `interval` before being written; Windows/Mac
  newlines are normalised to `\n`, so a later `GET /api/job/{name}` returns
  LF-only content. Omitting it truncates the job file to zero bytes and still
  returns `200 OK`.

> `include` and `block` are **not** resolved at this point, so content with a
> missing or cyclic `include` is saved with `200 OK` and only fails when the job
> is run (see `POST /api/job/{name}/run`). The file is written unconditionally
> at `<jobs dir>/<name>.yaml`, so posting to a name that has no file creates the
> job.

**Responses**: `200 OK` (empty body) on success; `400 Bad Request` for an
invalid job name, invalid YAML, an unparsable `interval`, or a write error
(error message in the body).

### DELETE /api/job/{name}

Delete the job file.

**Responses**: `200 OK` (empty body) on success; `400 Bad Request` if the job
name is invalid; `404 Not Found` (empty body) if the file does not exist;
`500 Internal Server Error` on a filesystem error.

### POST /api/job/{name}/run

Schedule a new build for the job and return the new build id as plain text.

**Input** (query parameters or form data — both are merged)

- any job parameter name — `string`, optional. Overrides the corresponding
  default parameter for this build. Only names declared in the job's `params:`
  are considered; unknown names are ignored, and an empty value is ignored too
  (the default is kept).

**Response** `200 OK` — the build id, e.g.:

```
123
```

**Other responses**

`400 Bad Request` with the error message in the body when:

- the job name is invalid, or no job with this name is registered —
  `invalid job name: <name>`;
- the job is disabled — `job <name> is not enabled`;
- the job file cannot be read or expanded at trigger time (it is re-read and
  re-parsed on every run): the file is gone, invalid YAML, a missing `include`
  file, or more than **1000** `include`/`block` expansions —
  `too many include/block expansions (1000) - check for a cyclic include`;
- the build itself cannot be created (database error, or its workspace,
  wakespace or artifacts directory cannot be written).

```bash
curl -u :admin -X POST 'http://localhost:8081/api/job/deploy-service/run?VERSION=1.2.3'
```

### POST /api/job/{name}/set_active

Enable or disable a job (a disabled job cannot be run or triggered by its
interval). Returns the new status as plain text.

**Input** (form data or query parameter)

- `active` — `string`, required. `"true"` or `"false"`.

**Response** `200 OK` — the new status (`true` / `false`), as plain text.
`400 Bad Request` if the job name is invalid. `500 Internal Server Error` for
any other value of `active` (body: `Invalid active flag for a job: <value>`), an
unknown job (body: `invalid job name: <name>`), or a database error.

---

## Build

### GET /api/build/{id}

Return everything needed to render a build page: the job definition snapshotted
when the build was created (`job`) and the latest runtime status
(`status_update`).

**Response** `200 OK`:

```json
{
  "job": {
    "name": "deploy-service",
    "desc": "Deploy the service",
    "tasks": [
      {
        "id": 0,
        "name": "Build",
        "run": "make build",
        "when": "",
        "if": "",
        "env": {},
        "status": "pending",
        "kind": "main",
        "logs": null,
        "include": "",
        "block": [],
        "ignore_errors": false
      }
    ],
    "defaultParams": [{ "VERSION": "main" }],
    "artifacts": ["output/*.tar.gz"],
    "interval": "@every 2h",
    "timeout": "10m",
    "concurrency": 0,
    "priority": 0
  },
  "status_update": {
    "id": 123,
    "name": "deploy-service",
    "status": "finished",
    "tasks": [
      {
        "id": 0,
        "status": "finished",
        "startedAt": "2026-06-13T10:30:45.123456+02:00",
        "duration": 5000000000,
        "kind": "main"
      }
    ],
    "params": [{ "VERSION": "1.2.3" }],
    "artifacts": ["output/service.tar.gz"],
    "build_artifacts": [{ "filename": "output/service.tar.gz", "size": 102400 }],
    "startedAt": "2026-06-13T10:30:40.000000+02:00",
    "duration": 10000000000,
    "eta": 9000000000
  }
}
```

> `job` is the build plan read back from `wakespace/{id}/build_plan.yaml`, so
> its empty collections come back as `[]` / `{}` rather than `null`, and
> `job.tasks[].status` is **always** the literal `"pending"` — the plan is never
> rewritten. Live per-task status, `startedAt` and `duration` come only from
> `status_update.tasks[]`, matched to `job.tasks[]` by `id`. `include` and
> `block` are always empty in the response: every `include:`/`block:` is
> expanded into the flat `tasks` list before the plan is stored. `logs` is
> always `null` — it is a placeholder the frontend fills in.

> Unlike `GET /api/feed`, this endpoint returns the stored status verbatim: a
> build left `pending`/`running` by a server restart keeps that status here until
> a `GET /api/feed` request re-classifies it as `aborted`.

**Responses**

- `200 OK` — the payload above.
- `404 Not Found` — empty body; the build's data directory still exists but the
  history has no entry for this id (a database that was reset or restored, or a
  cleanup that could not delete the build directory).
- `500 Internal Server Error` — the id is not an integer, or the build's stored
  job definition cannot be read. The body is the plain-text Go error, e.g.
  `strconv.Atoi: parsing "abc": invalid syntax` or
  `stat wakespace/999/build_plan.yaml: no such file or directory`.

> The job definition is read from disk *before* the database is queried, so an
> id that was never used — or one already removed by `buildHistorySize` cleanup,
> which deletes the directory and the history entry together — answers `500`,
> not `404`.

### POST /api/build/{id}/start

Start a queued build immediately, ignoring both the global `concurrentBuilds`
setting and the job's own `concurrency` limit. The build is removed from the
queue rather than reordered within it.

**Responses**

- `200 OK` — empty body.
- `404 Not Found` — empty body; the build is not queued (already running,
  already finished, or unknown id).
- `500 Internal Server Error` — the id is not an integer; the body is the
  plain-text Go error.

### POST /api/build/{id}/abort

Abort a queued or running build.

**Responses**

- `200 OK` — empty body. A queued build is moved to `aborted` asynchronously
  (its `on_aborted` and `finally` tasks still run); for a running build the
  abort is signalled to the task currently executing.
- `404 Not Found` — empty body; the build is neither queued nor running.
- `500 Internal Server Error` — the id is not an integer; the body is the
  plain-text Go error.

> The signal is best effort and is only picked up while a task's command is
> actually running. If the build is between tasks, or evaluating a `when`/`if`
> condition, the request still returns `200` and nothing happens — repeat the
> call. The job's `timeout` aborts through the same channel, so a build can
> occasionally outlive its timeout.

### POST /api/build/{id}/flush

Force the build to flush its buffered task logs to disk without stopping it
(useful to inspect logs of a long-running task).

**Responses**

- `200 OK` — empty body.
- `404 Not Found` — empty body; the build is not **running**. A build that is
  still queued (`pending`) also returns `404`.
- `500 Internal Server Error` — the id is not an integer; the body is the
  plain-text Go error.

> Like abort, the flush signal is dropped when no task command is executing at
> that instant; the response is still `200` and nothing is flushed.

### Retention

Builds are garbage-collected every 15 minutes: every build whose id is
`<= (newest build id) - buildHistorySize` is dropped from the history and its
`workspace/{id}` and `wakespace/{id}` directories are deleted. The build then
disappears from `/api/feed`, its artifacts and task logs stop being served under
`/storage`, and `GET /api/build/{id}` starts answering `500`. Builds that are
still queued or running are skipped and cleaned up on a later pass.

---

## Settings

### GET /api/settings

Return the application settings.

**Response** `200 OK`:

```json
{ "concurrentBuilds": 2, "buildHistorySize": 200 }
```

### POST /api/settings

Update the application settings.

**Input** (form data or query parameters)

- `password` — `string`, optional. When non-empty, sets a new password (stored
  bcrypt-hashed). Any non-empty value is accepted — there is no length or
  strength check — and an empty value is ignored, so the password cannot be
  cleared this way.
- `concurrentBuilds` — `integer`, required. Maximum number of builds running in
  parallel. On success the value is persisted and the queue is re-evaluated at
  once, so raising it can start queued builds immediately. Not validated — `0`
  or a negative value is accepted and stops any build from starting until the
  setting is raised again. This is the one field whose database error is never
  reported: the save only logs the failure and gives up, leaving both the stored
  value and the running limit unchanged and the queue not re-evaluated, and the
  handler — which gets no result to inspect — carries on to `buildHistorySize`
  and still answers `200` if that field succeeds. A `200` is therefore not proof
  that this field was saved; read it back with `GET /api/settings`.
- `buildHistorySize` — `integer`, required. Number of builds to keep in history;
  see [Retention](#retention) for what deleting a build removes. Not validated
  either — `0` drops the entire history, artifacts and logs included.

> Sending `password` in the query string puts it in URLs and proxy logs — send
> it in the form body.

**Responses**

- `200 OK` — empty body.
- `500 Internal Server Error` — `concurrentBuilds` or `buildHistorySize` missing
  or not an integer, a hashing error, or a database error. The body is the raw
  Go error, e.g. `strconv.Atoi: parsing "": invalid syntax`.

> Settings are applied one at a time, in the order password →
> `concurrentBuilds` → `buildHistorySize`, and the handler returns on the first
> *reported* failure. A `500` therefore does **not** mean nothing changed:
> posting only `password=...` stores the new password and *then* returns `500`
> because
> `concurrentBuilds` is missing. Always send all three fields.

> Changing the password takes effect immediately for HTTP Basic authentication
> but does **not** invalidate sessions: `session` cookies issued before the
> change stay valid for the rest of their 120 hours. Call `GET /auth/logout`
> from each browser to drop them.

```bash
curl -u :admin -X POST http://localhost:8081/api/settings \
  -d 'password=s3cret' -d 'concurrentBuilds=2' -d 'buildHistorySize=200'
```

---

## Build artifacts and logs (storage)

### GET (and HEAD) /storage/build/{path}

Serve files from a build's `wakespace` directory — the per-task logs, the
artifacts collected at the end of the build, and the build plan. Requires
authentication. Only `GET` and `HEAD` are routed; anything else returns
`405 Method Not Allowed`.

`{path}` is resolved relative to the server's `wakespace` directory, so it
always starts with the build id:

- `/storage/build/{id}/task_{taskID}.log` — the log of one task, where `taskID`
  is a `tasks[].id`. `.log` files are served as `text/plain` so they render in
  the browser instead of downloading. The file contains exactly the lines pushed
  over `build:log:<id>`.
- `/storage/build/{id}/artifacts/{filename}` — one collected artifact, where
  `{filename}` is a `build_artifacts[].filename` value. That value is the path
  **relative to the build workspace**, so it keeps the directory structure
  matched by the job's `artifacts:` patterns (pattern `output/*.tar.gz` gives
  `output/service.tar.gz`). A bare filename is *not* reachable directly under
  the `/storage/build/` prefix.
- `/storage/build/{id}/build_plan.yaml` — the fully expanded job definition the
  build ran.

Directory listing is enabled: `/storage/build/{id}/` returns an HTML index of
one build's files and `/storage/build/` an index of every build still on disk.
There is no JSON form of that listing — use `build_artifacts` from
`GET /api/build/{id}`.

```bash
curl -u :admin http://localhost:8081/storage/build/123/task_0.log
curl -u :admin -O http://localhost:8081/storage/build/123/artifacts/output/service.tar.gz
```

> Task logs are buffered while a build runs — call `POST /api/build/{id}/flush`
> first if you need the latest lines of a running task.

> Artifacts are copied into the build's storage through a secret-redacting
> filter that replaces every configured secret value with the literal
> `***REDACTED***`, so `build_artifacts[].size` is the size of that **stored
> copy** — which is what this endpoint serves — and may differ from the file the
> task left in the workspace. With no secrets configured the copy is
> byte-for-byte identical.

**Response headers.** `/storage/build/*` gets a Content-Security-Policy of its
own — loose enough to preview HTML artifacts, tightened where it matters most:

```
content-security-policy: default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'none'; form-action 'none'; frame-ancestors 'self'
referrer-policy: no-referrer
x-content-type-options: nosniff
```

Inline `<script>` and `<style>` in a self-contained report still run;
`fetch`/`XMLHttpRequest`/WebSocket (`connect-src 'none'`) and form submissions
(`form-action 'none'`) are blocked **for the artifact's own document**.

> Do not read that as a security boundary. Everything wakeci serves — the
> frontend, `/auth/*`, `/api/*`, `/docs/*` and `/storage/*` — is a single
> browser origin, and a Content-Security-Policy constrains the document it is
> served with, not the origin. `frame-src` is unset here, so framing falls back
> through `child-src` to `default-src 'self'`; every wakeci response allows
> `frame-ancestors 'self'` and none sets `X-Frame-Options`; the viewer's
> `session` cookie rides along on those same-origin requests, because
> `SameSite=Strict` only withholds cookies cross-site; and no CSP directive
> restricts top-level navigation. There is no CSRF token anywhere in the API.
>
> Treat a build artifact as untrusted code running with the viewer's session:
> this policy raises the bar for a careless artifact, it does not contain a
> deliberate one. If you preview artifacts you do not control, do it in a
> browser that is not logged in to wakeci, or serve them from a different host.

No `content-disposition` and no `cache-control` are set, so a browser renders a
file inline whenever its content type allows. The content type comes from the
file extension or, when the extension is unknown, from the server sniffing the
first 512 bytes — so an HTML report with an odd extension is still served as
`text/html`. `Range` requests are supported. Use `curl -O` (or a `download`
attribute) when you want a file saved rather than displayed.

**Status codes**: `200 OK`, `206 Partial Content` for a range request,
`404 Not Found` for an unknown path, plus the authentication responses `403` and
`429` described above.

---

## Static frontend

### GET /*

Everything that is not `/ws`, `/auth/*`, `/api/*`, `/storage/*` or `/docs/*` is
served from the embedded frontend bundle and **requires no authentication** (the
UI asks for credentials itself):

- a path containing a `.` is served from the bundle as-is (`/app.js` →
  `assets/app.js`) with `cache-control: public, max-age=604800, immutable`, and
  returns `404 Not Found` when the file is not in the bundle;
- every other path (`/`, `/build/123`, …) returns `index.html`, so the SPA can
  do client-side routing.

Only `GET` is routed here; other methods return `405 Method Not Allowed`.

---

## WebSocket API

### GET /ws

Real-time channel used by the frontend to stream build status changes and task
log lines.

**Upgrade handshake.** A normal WebSocket handshake: `GET /ws` with
`Connection: Upgrade`, `Upgrade: websocket` and `Sec-WebSocket-Version: 13`. The
connection is authenticated by the usual middleware during the upgrade, so the
same Basic-auth or `session`-cookie credentials apply.

The server also enforces a same-host `Origin` check:

- no `Origin` header (scripts, `curl`, non-browser clients) — accepted;
- one `Origin` whose host **and port** equal the request's `Host` header,
  compared case-insensitively (the scheme is not compared) — accepted;
- anything else — more than one `Origin` header, an `Origin` that does not parse
  into a host, or a different host/port — rejected with `403 Forbidden` and the
  plain-text body `Forbidden`. No WebSocket connection is established.

Browsers set `Origin` themselves, so a page must connect to its own origin
(`ws://`/`wss://` + `location.host` + `/ws`); pointing a dev page straight at the
backend port instead of through the dev proxy is rejected.

```bash
curl -i -u :admin \
  -H "Connection: Upgrade" -H "Upgrade: websocket" \
  -H "Sec-WebSocket-Version: 13" -H "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
  http://localhost:8081/ws
```

**Message envelope.** Every message (both directions) is a JSON object:

```json
{ "type": "<message type>", "data": <payload> }
```

A server-to-client text frame may contain **several** messages: whatever is
already queued for a client is coalesced into one frame, with the individual
JSON objects separated by `\n`. Split each incoming frame on newlines and parse
line by line rather than parsing the frame as a single object. Send exactly one
object per client-to-server frame — the server decodes only the first JSON value
in a frame and discards the rest.

**Subscribing.** Right after connecting, the client tells the server which
message types it wants. A subscription matches a message when the two strings
are **equal**, or — only when the subscription ends in `:` — when the message
type starts with it. Nothing else matches:

- `build:update:` — every build update (what the feed page uses);
- `build:` — every `build:update:*` **and** `build:log:*` message;
- `build:update:123` — build 123 only; it does **not** also deliver
  `build:update:1234`;
- `build:update` (no trailing colon) — matches nothing, with no error.

```json
{ "type": "in:subscribe",   "data": { "to": ["build:update:123", "build:log:123"] } }
{ "type": "in:unsubscribe", "data": { "to": ["build:log:123"] } }
```

`in:unsubscribe` removes an **exact** string: unsubscribing from
`build:update:123` does not cancel a `build:update:` namespace subscription.
Send back the same string you subscribed with. Subscribing twice to the same
string is a no-op.

Subscribe and unsubscribe are not acknowledged, and the two ways of getting a
message wrong behave differently:

- anything the server cannot decode as an envelope — invalid JSON, a non-string
  `type`, a message over 512 bytes — **ends the connection** without logging
  why; the client must reconnect and re-subscribe;
- a valid envelope always leaves the connection open. An unknown or missing
  `type` is logged as a warning; a `data` that cannot be decoded into
  `{"to": [...]}` — a `to` that is not an array of strings, a `data` that is a
  number, a string or an array, or no `data` key at all — is logged as an error.
  Either way the message is ignored.

> `data` of `null`, `{}`, `{"to": null}` or `{"to": []}` is **not** an error: it
> decodes to an empty list, so the server subscribes to nothing, logs nothing
> and answers nothing. Since subscriptions are never acknowledged, a
> subscription that quietly matched nothing looks exactly like one that worked —
> check that `to` really carries the strings you meant.

**Server-pushed messages**

- `build:update:<id>` — sent whenever a build or one of its tasks changes state.
  The payload is the same build object returned by `GET /api/feed`:

  ```json
  {
    "type": "build:update:123",
    "data": {
      "id": 123, "name": "deploy-service", "status": "running",
      "tasks": [
        { "id": 0, "status": "finished", "startedAt": "2026-06-13T14:30:45.123456+02:00", "duration": 5000000000, "kind": "main" },
        { "id": 1, "status": "running",  "startedAt": "2026-06-13T14:30:50.654321+02:00", "duration": 0,          "kind": "main" }
      ],
      "params": [{ "VERSION": "1.2.3" }],
      "artifacts": null, "build_artifacts": null,
      "startedAt": "2026-06-13T14:30:45.000000+02:00", "duration": 0, "eta": 45000000000
    }
  }
  ```

  A running build reports `duration: 0` — use `eta` and `startedAt` to render
  progress. `artifacts` and `build_artifacts` stay `null` until the build
  collects at least one artifact at the very end of the run.

- `build:log:<id>` — one message per task log line:

  ```json
  {
    "type": "build:log:123",
    "data": { "taskID": 1, "id": 0, "data": "[      50ms] Building...\n" }
  }
  ```

  `taskID` identifies the task that produced the line. `data` is the formatted
  log line: `[`, the elapsed time since the task started right-aligned in 10
  characters, `] `, then the line itself with ANSI colour codes stripped and
  secrets redacted, terminated by `\n` — byte-for-byte what is written to
  `/storage/build/<id>/task_<taskID>.log`. Lines wakeci emits itself start with
  `>` after that prefix, e.g. `[      12ms] > Aborted by a user.\n`.

  Carriage-return redraws (progress bars) are collapsed before sending: a
  trailing `\r` is dropped and, of what remains, only the text after the last
  `\r` is kept — so a bar that redrew 500 times arrives as one line in its final
  state.

  `id` is always `0`. It is an unused placeholder, **not** a sequence number;
  use the arrival order of the messages instead.

**Keep-alive and limits**

- The server sends a ping every **54 s**. The read deadline starts at **60 s**
  and is pushed forward by each pong — and only by a pong; sending data does not
  extend it, so a client that never answers a ping is disconnected. Browsers
  pong automatically; a custom client must too.
- Each server write must complete within **10 s**.
- Incoming client messages are limited to **512 bytes**. A larger message is not
  truncated: the server replies with a close frame of code `1009` (message too
  big) and drops the connection, so split a long `in:subscribe` list across
  several messages.
- Each connection has a 1024-message outbound buffer. A client that does not
  read fast enough to keep that buffer from filling is dropped with a plain
  close frame and no error message, and must reconnect and re-subscribe.

---

## Interactive documentation

- `GET /docs/api/` — Swagger UI for the REST API. The trailing slash is
  **required**; `/docs/api` returns `404`. The page loads
  `swagger-ui-dist@4.4.0` from `unpkg.com`, so the browser needs access to that
  CDN to render it.
- `GET /docs/swagger.json` — the raw OpenAPI/Swagger 2.0 specification,
  generated at build time and embedded into the binary.

> Unlike `/ws`, `/api`, `/storage` and `/auth/_isLoggedIn`, the `/docs` prefix is
> **not** behind authentication: anyone who can reach the instance can read the
> API reference.

> The spec is generated from annotations that are not kept in step with the
> router, so treat it as a convenience rather than a contract:
> `POST /api/build/{id}/start` is absent from it (its handler carries the
> `/build/{id}/abort` annotation), several paths are spelled with a trailing
> slash the router rejects — `/api/settings/`, `/api/job/{name}/` — and the
> `400`/`429` responses are not annotated. It also declares no security scheme,
> so Swagger UI sends no credentials and "Try it out" returns `403` unless the
> browser already holds a `session` cookie.
