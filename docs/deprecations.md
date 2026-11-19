# Deprecations

What still works but is going away, what replaces it, who still uses it,
and what removing it touches. Nothing is deprecated right now.

## How to deprecate something

Keep the old parameter or field working, document it as deprecated where
it is described (its `@Param` or field comment, the README), and give it a
section here:

- **Replaced by:** what clients should use instead.
- **Why:** what was wrong with it.
- **Still used by:** every known client, and how you know.
- **To remove:** the code, tests and docs that go, and what an old client
  gets afterwards. An error that names the replacement is better than a
  parameter that is silently ignored.

Remove it once nothing uses it; the request log shows who still sends a
query parameter.

## Check who still sends a parameter

The request line (`msg="request completed"`, always on, INFO) shows the
query parameters in `loggedQueryParams` (`middleware.go`) for reads, and
who sent them (`auth`, `user`, `apiKey`, `ua`). For a parameter `NAME`, on
each server:

```bash
journalctl -u wakeci --since -7d -o cat \
  | grep 'msg="request completed"' \
  | grep -E 'query="[^"]*\bNAME=' \
  | grep -oE '(path|auth|user|apiKey|ua|status)=("[^"]*"|[^ ]*)' | sort | uniq -c | sort -rn
```

Every program has its own API key, so `apiKey` names the caller. A response
field can't be checked this way: ask the owners of the scripts that read it.

## Removed

- **2026-11:** `offset` and `filter` on `GET /api/feed`, and `artifacts` in
  build updates. `offset` and `filter` get 400 with a message naming
  `before` and `q` (`removedFeedParams` in `handlers_feed.go`), so an old
  client fails instead of reading the first page again and again or an
  unfiltered feed; they stay in `loggedQueryParams` so the request log shows
  who still sends them. Old feed links with `?filter=` open the unfiltered
  feed. Builds carry only `build_artifacts`; history entries saved before
  keep `artifacts` in the database, which is never sent.
