# The wake command

Tasks start other builds, and read wakeci's API, with `wake`. It is the
only way a task needs: no API key in the job, no `curl`, no script that
retries. The builds it starts are shown under the task's build in the feed
and on the build page.

```yaml
tasks:
  - name: Test every service
    run: wake start test_api test_web --inherit IMAGE="$(<image)"
  - name: Check every node
    run: list_nodes | wake start check_node --each NODE
  - name: Deploy, one node at a time
    run: list_nodes prod | wake start deploy --each NODE --wait VERSION=$VERSION
```

## In a task

wakeci links its own binary as `wake` into `<workdir>/bin` at startup and
puts that directory first on every task's `PATH` (`installWakeCommand`,
`wakeCommandDir`), so there is nothing to install and its version always
matches the server's. Tasks also get:

- `WAKE_URL`, the server;
- `WAKE_TOKEN`, the build's token (see [Build tokens](#build-tokens));
- `WAKE_JOB_PARAMS`, the build's parameters, which `--inherit` passes on.

If wakeci can't create the link, it logs a warning and runs everything else.

## wake start

```
wake start JOB... [NAME=VALUE...] [options]
```

Starts a build of each job, in the order given. An argument with `=` is a
parameter; anything else is a job. Options may come anywhere; `--` ends
them.

| Option | |
|---|---|
| `--inherit` | Pass on this build's parameters that the started job has. `NAME=VALUE` changes one; `NAME=` leaves it at the job's default. |
| `--each NAME` | Start one build per line of standard input, with the line as `NAME`. Blank lines are left out; no lines start nothing (exit 0). |
| `--params-file F` | Parameters from the file `F`, one `NAME=VALUE` a line; blank lines and lines starting with `#` are left out. |
| `--wait` | Start the builds one at a time, each once the one before has ended, its hooks included, and stop at the first that didn't finish. |
| `--again ID` | Run build `ID` again: its job, with its parameters, which `NAME=VALUE` may change. The new build keeps that build's parent, as Run again does. |

Parameters come in this order, later ones winning: inherited, the build
run again, the file, the arguments, the `--each` line.

**Everything is checked before the first build starts**: that each job
exists and is enabled, that each parameter is one the job has, and, for
callers who may not edit jobs (a build's token, a key without `write`), that
no value breaks the rule for untrusted parameters. If anything is wrong,
`wake` prints every problem and `Nothing was started.`, so a typo in a job
name or in line 40 of a list starts nothing.

For each build it prints `Started JOB #ID: URL`; with `--wait` also
`JOB #ID: STATUS` when the build has ended. When a start fails, it prints
why and `Not started: ...` for each build after it.

A start is retried, after 2, 4, 8, 16 and 30 seconds, only while the server
certainly took nothing: it couldn't be reached, it answered 429 (waiting at
least its `Retry-After`), 502, 503 or 504, or it said it is stopping. Once a
request may have reached the server, it is never sent again, so a retry
can't start a build twice; `wake` then says the build may have started.

Exit status: 0 if every build started (and, with `--wait`, finished), 1 if
something failed or nothing was started, 2 if `wake` was called wrongly.

## wake api

```
wake api PATH
```

Prints the answer to `GET PATH`, for example
`wake api /api/build/$WAKE_BUILD_ID` or `wake api "/api/feed?q=+deploy"`. It
exits with 1 and prints the status and answer if the server refuses. The
path is always below `WAKE_URL`, so the token is never sent anywhere else.
Every route is in `/llms-full.txt`.

## Builds that replace older ones

A push makes the builds of the push before it useless. With a cancel group,
the newer build cancels them (`cancel.go`):

```yaml
cancel_group: tests/$BRANCH
cancel_unless: '[[ $BRANCH =~ ^(main|release)$ ]]'
```

- As a build is queued (`Queue.Add`), it aborts the older builds, queued or
  running, whose key is the same, of every job with that group, and the
  builds they started (`startedUnder`). Their on_aborted and finally tasks
  run, and their log says "Aborted as superseded by #N (tests/feature)".
- The later build wins: both are looked for under the queue's lock, so a
  build queued after a newer one with its key cancels itself. There is no
  check of which commit is newer.
- Builds of one group never cancel each other, so a build's children can
  share its key.
- A key that names a param without a value is no key, and while
  `cancel_unless` succeeds (run with the params as variables) the build
  cancels nothing.
- The run's audit event lists what it `cancelled`, or who it was
  `supersededBy`.

When the builds share something, such as a test server, the newer one
should start only after the older one's cleanup. `concurrency_key` makes
`concurrency` (1 if unset) count the builds of every job with that key,
also those running their hooks after an abort (`Queue.concurrencyFull`):

```yaml
cancel_group: pr_init/$NAME
concurrency_key: pr_init/$NAME
```

## Build tokens

Every build gets a token, `wb_<build ID>_<secret>`, when its first task
starts (`buildtoken.go`). The key router takes it like an API key
(`routeAPIKeys`, `buildTokenAuth`):

- It may **read and run every job**, but not change jobs, and never opens
  settings, API keys, login or the websocket. Every job, because a job's
  commands run as wakeci's user and can already read its database and
  secrets; limiting the token would add no protection.
- The builds it starts are its build's children, without `WAKE_PARENT`; a
  different `WAKE_PARENT` is refused.
- **An abort stops it starting builds** until the build's on_aborted and
  finally tasks run, which may start builds again (`RunJob`). A build its
  tasks started as the abort came is aborted too ("Aborted as part of #N,
  which was aborted"), so a cancel group can't miss one that was on its
  way. A run with `WAKE_PARENT` follows the same rule.
- It can't edit jobs, so the builds it starts follow the rules for untrusted
  parameters ([docs/access.md](access.md)), as builds started with a run key
  always did.
- It **stops working when its build leaves the queue**, its hooks done. It
  is kept in memory only, so a restart gives the builds it restores new
  tokens.
- Build logs, summaries and artifacts redact the tokens of the builds in the
  queue like secrets (`buildTokenValues` in `secretRedactionValues`), so a
  reader of a log can't act as the build while it runs.
- The audit log names the build (`auth=build byBuild=12`), the request line
  too, and the builds it starts count as started by "Other builds" on the
  analytics page (`TriggerBuild`). Failed tokens count on the same limiter as
  failed API keys; each token has its own request limit.

## On another wakeci server

A task starts a build on another server by pointing `wake` there with an
API key of that server, kept in the secrets file:

```yaml
  - name: Deploy staging server
    env:
      WAKE_URL: https://build.example.com/
      WAKE_TOKEN: "{{ secrets.BUILD_WAKE_KEY }}"
    run: wake start pr_init NAME=pr$(<pr_number)
```

The task's `env` comes after wakeci's variables, so it wins. The other
server sees the key, so the build has no parent there and the key's scopes
and jobs apply; give it `run` on the jobs it starts only. The key is a
secret, so logs redact it; the task's processes can still read it, as any
secret a task uses.

## From your own machine

`wake` is the wakeci binary under another name:

```bash
ln -s /usr/local/bin/wakeci ~/bin/wake
export WAKE_URL=https://ci.example.com/ WAKE_TOKEN=wk_...   # an API key
wake start deploy VERSION=1.2.3 --wait
```

With an API key, builds have no parent and the key's scopes and jobs apply.

## Instead of curl

| Before | With wake |
|---|---|
| `curl -X POST -H "Authorization: Bearer {{ secrets.KEY }}" "${WAKE_URL}api/job/deploy/run?WAKE_PARENT=${WAKE_BUILD_ID}&VERSION=$V"` | `wake start deploy VERSION=$V` |
| A script that copies `$WAKE_JOB_PARAMS` and changes some | `wake start child --inherit IMAGE="$(<image)" CACHE=` |
| `for n in $(nodes); do curl ...; done`, which hides every failed start but the last | `nodes \| wake start check --each NODE` |
| A script that starts deploys one by one and stops at the first failure | `nodes \| wake start deploy --each NODE --wait` |
| A script that reads the feed with a key and re-runs failed builds | `wake api ...` and `wake start --again ID` |
