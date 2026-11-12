# Deprecations

What still works but is going away, what replaces it, who still uses it,
and what removing it touches. Planned removal: the week of 2026-11-19, for
each item whose users have moved. Check the request log first; an item
stays until nothing sends it.

## Check who still uses a parameter

The request line (`msg="request completed"`, always on, INFO) shows the
`offset` and `filter` query parameters, and who sent them (`auth`, `user`,
`apiKey`, `ua`). On a server:

```bash
journalctl -u wakeci --since -7d -o cat \
  | grep 'msg="request completed"' \
  | grep -E 'query="[^"]*\b(offset|filter)=' \
  | grep -oE '(path|auth|user|apiKey|ua)=("[^"]*"|[^ ]*)' | sort | uniq -c | sort -rn
```

Servers that run a release before this change log requests at WARN
without the query, so the check only means something after the update.

## `offset` on `GET /api/feed`

- **Replaced by:** `before=<the oldest ID you got>` to page back, and
  `after=<the newest ID you have>` to poll for new builds.
- **Why:** `offset` counts builds, so a client that counts differently from
  the server, or a build that arrives or goes between two pages, repeats or
  skips builds. With a search, the server also reads and throws away
  `offset` matching builds on every page, while `before` jumps to the ID.
- **Still used by:** wakesync (`roles/prometheus/files/wakesync.py` in the
  deployment repo) on ti-ci and int-build, until it reads with
  `after=<lowest unfinished or newest stored ID>&limit=200`. The feed page
  used it until it switched to `before`; browser tabs opened before the
  update send it until they reload.
- **To remove:** `feedRequest.offset`, its parsing and the skip in
  `HandleFeedView` (`src/backend/handlers_feed.go`), its `@Param`,
  `"offset"` in `loggedQueryParams` (`middleware.go`), and the cases that use
  it in `handlers_feed_test.go`, `apikeys_test.go`, `handlers_build_test.go`
  and `request_log_test.go`. Decide whether an old client then gets 400 (an
  unknown parameter is ignored today, so it would silently get the first
  page again and again).

## `filter` on `GET /api/feed` and in feed links

- **Replaced by:** `q`, in the API and in the page URL (`/?q=...`).
- **Why:** `filter` was the old name. Links to the feed page also encoded it
  twice.
- **Still read:** by `HandleFeedView` when `q` is absent, and by `FeedView`
  for links written before, which it decodes once more and rewrites to `q`.
- **Still used by:** unknown; the request log shows API clients, and page
  loads of `/` with `filter=` show old links being opened.
- **To remove:** the fallback in `feedRequestFrom` (`handlers_feed.go`), its
  `@Param`, `"filter"` in `loggedQueryParams`, the `filter` branch in
  `FeedView.vue`'s `mounted`, the `filter` cases in `handlers_feed_test.go`
  and the E2E test "should take q over an old filter parameter", and the
  Gotcha in `AGENTS.md`. Old bookmarks then open the unfiltered feed.

## `artifacts` in build updates

- **Replaced by:** `build_artifacts`, which has each file's name, size and
  URL. The field is in the feed, `GET /api/build/{id}` (`status_update`) and
  the WebSocket build updates. Not to be confused with `artifacts` in
  `GET /api/build/{id}/summary`, which already holds the full entries.
- **Why:** it holds file names only, and every build stores the list twice.
- **Still used by:** `BuildView.vue`, only for builds saved before
  `build_artifacts` existed (January 2021); `buildHistorySize` has most
  likely removed those. The request log can't show which clients read a
  response field, so ask the owners of scripts that read builds.
- **To remove:** `Build.Artifacts` and the lines that fill it
  (`src/backend/build.go`), `BuildUpdateData.Artifacts`
  (`communication.go`), and the fallback in `BuildView.vue`'s
  `getArtifacts`. History entries keep the old field in the database, which
  is harmless.
