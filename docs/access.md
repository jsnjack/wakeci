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
  the settings, matched ignoring case.
- **An API key** may do what its own grant allows. A key made by a single
  sign-on user is also limited by what the creator's groups allow now
  (`Access.Within`), so it never does more than its creator. The key records
  the creator's groups (`CreatorGroups`), and every login of the creator
  records them again, so a person who left a group in Google loses it on
  their keys at their next login. Keys made with the admin password, and
  keys made before wakeci recorded the creator's groups (`CreatorGroups` is
  null), are limited by their grant only, the latter until their creator
  logs in.

## Groups

Groups live in the `ssogroups` bucket under their lowercased name, each with
its grant. Only the admin password adds, changes and removes them
(`/api/groups/`, `requireAdmin`); a user with `write` sees the list
read-only. A login is accepted when one of the groups the identity provider
sends exists. The settings page lists the groups and edits them with the
same picker as new API keys (`AccessPicker.vue`).

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

Each API key may send a burst of 120 requests, refilled at 10 a second
(`apiKeyRequestLimiter`, after the key is verified); beyond that it gets
429 with `Retry-After`. That is far above what clients send, so it only
slows down a loop that went wrong, and it is per key, so one key can't use
up another's requests.

The access is computed from the groups as they are on every request, not
kept in the session, so a change in the settings applies to the members'
next request. A user whose groups now allow nothing gets 403 from `AuthMi`,
and the page logs out, as it does on any 401. Sessions keep the groups the identity provider sent
(`session.groups`); a single sign-on session saved before sessions kept
them is ended when wakeci loads it, so that user logs in once more.

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
person gets what their groups allow.
