# Access: who may do what

Read this before changing single sign-on groups, API key scopes, a route's
access check, or what the live updates send to whom.

## The model

A grant is a set of scopes on a set of jobs (`Grant` in `access.go`). The
scopes are `read` (lists, job files, builds, logs, artifacts), `run` (run,
abort, start, flush) and `write` (create, edit, delete, enable). They don't
include each other. A grant covers all jobs, including jobs added later, or
jobs by exact name and by name patterns in which `*` matches anything.
`write` only counts on a grant that covers all jobs, because a job runs
commands on the server and can reach every other job and secret.

What a request may do is an `Access`:

- **The admin password** (a password session or Basic auth) may do
  everything: `fullAccess`.
- **A single sign-on user** may do what the grants of their groups allow,
  united: a scope on a job is allowed if any of their groups allows it.
  Their groups are those the identity provider sent at login that exist in
  the settings, matched ignoring case. Someone in none of them gets the
  rule for people in no group, if the admin set one, and otherwise can't
  sign in.
- **An API key** may do what its own grant allows. A key made by a single
  sign-on user is also limited by what the creator's groups allow now
  (`Access.Within`), so it never does more than its creator. The key records
  the creator's groups (`CreatorGroups`), and every login of the creator
  records them again, so a person who left a group in Google loses it on
  their keys at their next login. A login refused because none of the
  person's groups is allowed records them too (`[]` for none, never null),
  so their keys can then do only what the rule for people in no group
  allows, which is nothing while it is off. Keys made with the admin password, and
  keys made before wakeci recorded the creator's groups (`CreatorGroups` is
  null), are limited by their grant only, the latter until their creator
  logs in.

## Groups

Groups live in the `ssogroups` bucket under their lowercased name, each with
its grant. Only the admin password sees, adds, changes and removes them
(`/api/groups/`, `requireAdmin`); for anyone else the Settings page has no
groups section. A login is accepted when one of the groups the identity
provider sends exists, or when the rule for people in no group is set. The
section lists the groups, with how many people
with a current session are in each (`SessionStorage.GroupMembers`), and
edits them with the same picker as new API keys (`AccessPicker.vue`). It
shows before single sign-on is set up, with a note, so the groups can be
ready when it is.

The rule for people in no group is a grant of its own, stored as an
`SSOGroup` without a name under `ssoNoGroup` in the `global` bucket, so no
group's name can stand for it (`/api/nogroup`, `requireAdmin`; `noGroup` in
`/api/groups/`, null while it is off). `ssoGroupsAccess` applies it only
when none of a person's groups exists: a member of a group gets only the
groups' permissions, even if the rule allows more, so a narrow group never
loses to an outsider. It covers people the identity provider sends no
groups for too, and their API keys (`CreatorGroups` that match no group).
It is off until the admin allows it; it may give `write`, with a warning in
the editor, since everyone the identity provider lets in could then run
commands. Turning it off works like removing a group: the next request of
those it covered is refused, and their live connections close.

`sso.groups` in Wakefile.yaml adds its groups with every scope on all jobs on
the first start that lists any, and records that in `ssoGroupsSeeded` in the
`global` bucket. After that the config is ignored, so a group the admin
removed doesn't come back; wakeci logs a warning at startup while
`sso.groups` is still set.

## Where it is checked

`AuthMi` and `APIKeyAuthMi` put the request's `Access` in its context.
Every route then says what it needs with `requireAccess(scope, jobResolver)`
on both routers, the same way: the job comes from `{name}`, or from the
build's history entry for `/build/{id}` and `/storage/build/{id}/...`. A
request without an `Access` in its context is refused, so a route can't be
opened by a middleware that forgot to set one. Settings and updates need
`write`, since `write` can run any command on the server anyway; the queue
status and a user's own keys are open to every logged-in user.
`TestEveryAPIRouteChecksAccess` walks `apiRoutes` with a user whose grant
covers no real job and fails for any route that lets them through and isn't
listed as open.

Missing or wrong credentials get 401 (`refuseCredentials`), and a refusal
of what was asked for gets 403, so a client can tell "log in again" from
"not allowed". The 401 carries a Basic challenge only when the request sent
Basic auth: a browser shows its own password dialog for a Basic challenge
on the page's own requests.

The feed and the job list filter themselves with `Access.Reads`.

Each API key may send a burst of requests, refilled at a steady rate
(`apiKeyRequestLimiter`, after the key is verified); beyond that it gets
429 with `Retry-After`. Both are settings (`apiKeyBurst`, `apiKeyRate`;
120 at once and 10 a second unless changed), applied as soon as they are
saved and when wakeci starts; a client that saves the settings without
them keeps their values. The defaults are far above what clients send, so
they only slow down a loop that went wrong, and the limit is per key, so
one key can't use up another's requests.

Each key counts the requests that pass these checks, in total and per day
for the last 30 days (`Uses`, `UsesByDay`, the chart on the API keys page),
so the admin can see whether something still uses a key before revoking
it. The counts are kept in memory (`key_usage.go`) and saved with the
key's last use, which `touchAPIKey` writes at most once a minute, and by
`flushAPIKeyUses` when wakeci stops; the list adds what isn't saved yet, so
it is exact, and a crash loses at most a minute of counts. An MCP tool's
call of a route carries `mcpCallKey`, so it is checked but neither counted
nor limited again: a tool call is one request.

The access is computed from the groups as they are on every request, not
kept in the session, so a change in the settings applies to the members'
next request. A user whose groups now allow nothing gets 403 from `AuthMi`,
and the page logs out, as it does on any 401. Sessions keep the groups the identity provider sent
(`session.groups`); a single sign-on session saved before sessions kept
them is ended when wakeci loads it, so that user logs in once more.

## MCP

`/mcp` (`mcp.go`) serves the API as MCP tools, on the key router only, so
it needs an API key. It is stateless: one HTTP request per call, no
session to re-check after a revoke. Each tool makes the matching API
request to the key router in-process, with the caller's key, address and
a `wakeci-mcp` User-Agent, so scopes, jobs, `rejectUnsafeParams`, the
audit log, the request limit and the error messages are those of the API.
The inner request gets a fresh context (only cancellation follows), since
the MCP request's carries its chi route and audit record.

## Live updates

A WebSocket client keeps the access it was opened with. The hub sends a
build's updates and log lines (`MsgBroadcast.Job`, which is never sent to
the page) only to clients that may read the job, and the queue status to
everyone; a message about a job that doesn't name it goes to the admin
password only. After a group changes, `onSSOAccessChange` calls
`Hub.CloseRestricted`, which closes every connection that wasn't opened with
the admin password, and the page opens it again at once with the new access.

## The pages

`/auth/_isLoggedIn` returns the user, whether it is the admin password, and
the grants. The store's `can(scope, job)` and `canAny(scope)` mirror the
backend's rules to hide what the user can't use: run and abort buttons,
editing, the settings link. The API checks every request regardless. A 401
logs the page out. A 403 logs it out only if `/auth/_isLoggedIn` fails too;
otherwise the request was something the user's groups don't allow, and the
page shows the error and reads the permissions again.

## What it doesn't do

Group membership comes from the identity provider at login, so a person
removed from a group in Google keeps it until their session ends (12 hours)
or the admin revokes their access. There are no per-user permissions: a
person gets what their groups allow, or, in none of them, what the rule for
people in no group allows.
