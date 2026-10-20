# wakeci

### What is it?

wakeci is an automation tool which helps to execute repetitive tasks

![ScreenShot](https://raw.githubusercontent.com/jsnjack/wakeci/master/screenshots/feed.png)

### Features

-   simple job configuration using YAML files with Ansible inspired syntax
-   easy to install - just download a binary file from Releases
-   automatic Let's Encrypt SSL certificates
-   parameterized builds, artifacts, intervals and timeouts - see job configuration example below
-   no plugins, no extensive configuration - focus on your project instead!

### Job configuration example

> See full description [here](https://github.com/jsnjack/wakeci/blob/master/src/frontend/src/assets/configDescription.yaml)

```yaml
desc: Build and release wake application
params:
    - VERSION: master

tasks:
    - name: Clone repository
      run: git clone git@github.com:jsnjack/wakeci.git --recursive

    - name: Checkout version
      run: sh ${WAKE_CONFIG_DIR}utils/checkout.sh wakeci ${VERSION}

    - name: Install npm dependencies
      run: cd wakeci/src/frontend && npm install

    - name: Build application
      run: cd wakeci && make build

    - name: Create a release on github
      run: python ${WAKE_CONFIG_DIR}utils/release_on_github.py -f wakeci/bin/wakeci -r jsnjack/wakeci -t "v`cd wakeci && monova`"

timeout: 10m

on_failed:
    - name: Send notification to Slack
      run: >-
          python ${WAKE_CONFIG_DIR}utils/notify_slack.py
          -t "Job ${WAKE_JOB_NAME} has failed <${WAKE_URL}build/${WAKE_BUILD_ID}|#${WAKE_BUILD_ID}>"
          -k error

on_finished:
    - name: Send notification to Slack
      run: >-
          python ${WAKE_CONFIG_DIR}utils/notify_slack.py
          -t "New wake version `cd wakeci && monova` <${WAKE_URL}build/${WAKE_BUILD_ID}|#${WAKE_BUILD_ID}>"
          -k ok
```

### How to use it?

```
Usage of ./bin/wakeci:
  -V	Print the version and exit (shorthand).
  -compactdb
    	Reclaim space in the database which is no longer used
  -config string
    	Configuration file location (default "Wakefile.yaml")
  -version
    	Print the version and exit.
```

#### Wakefile.yaml format

```
# Port to start the server on (default "8081")
port: 8081
# Hostname for autocert. Active only when port is 443
hostname: ""
# Working directory (default ".wakeci/")
workdir: ./wakeci
# Configuration directory - all your job files (default "./")
jobdir: ./
# Path to the file with secrets
secretsfile: ./secrets.yaml
# Scheduled jobs (via `interval` field) will use this timezone, if not specified
# in the job configuration
timezone: Europe/Amsterdam
# Optional "Sign in with Google" for Google Workspace accounts, next to the
# shared password
google:
  client_id: 1234-abc.apps.googleusercontent.com
  client_secret: GOCSPX-...
  # Required. Only accounts of these Google Workspace domains can sign in;
  # personal Google accounts never can
  allowed_domains: [example.com]
  # Optional, defaults to <scheme>://<hostname or request host>/auth/google/callback
  # redirect_url: https://ci.example.com/auth/google/callback
  # Optional: turn the shared password off, leaving Google as the only way in
  # disable_password: true
```

> Default password is `admin`. Don't forget to immediately change it!

### Authentication

- **Password**: one shared password (Settings page), for the UI and for basic
  auth API calls (`curl -u :password ...`). Has full access, and keeps
  working next to Google sign-in. A shared password can't be offboarded:
  anyone who knows it keeps access after leaving, so once everyone uses
  Google, set `google.disable_password: true`. That turns it off entirely,
  along with its sessions and the API keys created with it. If the Google
  setup breaks, unset it and restart to get the password back. Changing the
  password signs out all password sessions, including your own, revokes all
  password-owned API keys, and ends their previews and live-update connections.
  Google sessions remain active. When Google sign-in is enabled, the default
  `admin` password is refused; set a new password before enabling Google if
  you need password access during migration.
- **Google**: for Google Workspace accounts only. In the Google Cloud console
  of your Workspace organization, set the OAuth consent screen's user type
  to **Internal** (Google then rejects anyone outside your organization), and
  create an OAuth client ("Web application") with
  `https://<your host>/auth/google/callback` as an authorized redirect URI.
  Then set the `google` block above. Google users get full access in the UI.
- **Offboarding is automatic**: wakeci asks Google every 5 minutes whether
  each Google user is still active. When Google refuses (the account was
  suspended or deleted, or the user revoked wakeci's access) or the account
  left the allowed domains, the user's API keys are deleted, their sessions
  ended and their open live-update connections closed. If Google can't
  confirm an account for 30 minutes, its keys and sessions are refused until
  it can.
- **API keys**: any logged-in user can create keys on the API keys page and
  pick their scopes. Send a key as `Authorization: Bearer wcik_...` or as the
  basic auth password. Keys can only be created, listed and revoked from a
  logged-in browser session.
- **Job access**: a key can be limited to some jobs: pick them from the list,
  or match them by name with patterns like `test_*` (`*` matches anything),
  which also cover matching jobs added later. Such a key only sees and acts
  on those jobs and their builds, logs and artifacts; the feed, the jobs list
  and live updates leave the rest out. Over the API, repeat
  `job=<name or pattern>` when creating the key; without it, a key covers
  every job. Keys carrying `write` must cover all jobs; old restricted write
  keys are refused and must be recreated.

| Scope   | Allows |
|---------|--------|
| `read`  | Feed, jobs, job configs, builds, logs, artifacts, websocket updates |
| `run`   | Run jobs, start and abort builds |
| `write` | Create, edit, delete and enable/disable jobs. Jobs run commands on the server (also on a schedule), so treat a `write` key like access to the server |
| `admin` | Settings, including the shared password. Only logged-in users and the shared password have it; it can't be granted to API keys |

### Security boundaries

Build HTML is displayed in a sandbox without access to wakeci's authenticated
origin. Preview URLs expire after 15 minutes, grant only the report directory
(or the build's artifacts directory), and stop working when their issuing
session or key is revoked or their password changes. Relative report assets
can load through the preview URL; authenticated app APIs, popups, and modal
dialogs are unavailable. Preview symlinks cannot escape the signed directory
inside the build.
Preview URLs are credentials: share them only with people allowed to read
those report files.

Only values written in job definitions expand `{{ secrets.NAME }}`. Parameters
submitted when running a job and values written into `build.env` stay literal.
The active config and secrets files, including filesystem aliases, cannot be
registered or accessed as jobs. Stale registrations for other YAML are removed
at startup and when files change. Job routes check the current file as well
as its registration. Job files must define `tasks`; use `tasks: []`
for an empty job. Saves replace job files atomically. Disabled state survives
temporary file removal or invalidation. Explicitly deleting a job through the
UI or API clears its saved state, so recreating that name starts a fresh job. Jobs execute as the wakeci process: job authors
and holders of `write` keys must be trusted with the server and its secrets.

State-changing browser requests require the same origin and a request body
of at most 4 MiB. Logout uses `POST /auth/logout`. Passwords are accepted only
in form bodies, never query strings. Keep a TLS reverse proxy configured to
replace forwarded headers and deny direct public access to the backend.

Credential responses and build files are not cached. The work directory is
owner-only, database copies and fresh trace logs use mode `0600`, and the
Google account checker refuses access after 30 minutes without confirmation.
Automated Google tests use a mock endpoint; validate suspension and revocation
against a real Workspace tenant before relying on automatic offboarding.

### API documentation

See full description [here](https://github.com/jsnjack/wakeci/blob/master/API.md)

### Development

Requires golang 1.26+

#### How to install golang 1.26+

```bash
go install golang.org/dl/go1.26.0@latest
"$(go env GOPATH)/bin/go1.26.0" download
# Manage different versions with `alternatives`
sudo alternatives --install /usr/bin/go go "$(go env GOPATH)/bin/go1.26.0" 10
# Switch between different go versions
sudo alternatives --config go
```

> Golang downloads page https://golang.org/dl/

#### Install dependencies

```bash
sudo dnf install entr
go install github.com/swaggo/swag/cmd/swag@latest

# cd src/frontend
npm install
```

#### Start application

```bash
# frontend
make runf

# backend
make runb
```

#### Update all outdated npm packages

```bash
npm outdated --json | jq -r 'keys[]' | xargs -I % npm i %@latest --save
```

#### Update all outdated go packages

```bash
GOWORK=off go-mod-upgrade
```
