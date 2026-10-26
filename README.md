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
# Hostname for autocert (active only when port is 443) and the default Google
# sign-in redirect URL
hostname: ""
# Working directory (default ".wakeci/")
workdir: ./wakeci
# Configuration directory - all your job files (default "./"). Every .yaml file
# in it can be opened as a job, so keep this file and the secrets file outside it
jobdir: ./jobs
# Path to the file with secrets
secretsfile: ./secrets.yaml
# Scheduled jobs (via `interval` field) will use this timezone, if not specified
# in the job configuration
timezone: Europe/Amsterdam
# Optional "Sign in with Google" for one Google Workspace domain, next to the
# password. Create an OAuth client of type "Web application" in Google Cloud
# and register the redirect URL on it.
google:
  clientid: 1234.apps.googleusercontent.com
  clientsecret: your-client-secret
  # Only accounts of this Workspace domain can log in
  domain: example.com
  # Defaults to https://<hostname>/auth/google/callback
  redirecturl: ""
```

Google sign-in sessions last 12 hours, password sessions 5 days. API calls
keep using Basic auth with the password.

Every account in `google.domain` can log in. To limit sign-in to a group, set
the OAuth consent screen to "Internal" and restrict the OAuth client to that
group in the Google Admin console (Security, API controls, Manage third-party
app access). Open wakeci under the same host as `redirecturl`; the sign-in
cookie is tied to that host, so a login started from another host name fails.

> Default password is `admin`. Don't forget to immediately change it!

### API documentation

See full description [here](https://github.com/jsnjack/wakeci/blob/master/API.md)

### Development

Requires golang 1.18+

#### How to install golang 1.18+

```bash
go get golang.org/dl/go1.18.2
/home/$USER/go/bin/go1.18.2 download
# Manage different versions with `alternatives`
sudo alternatives --install /usr/bin/go go /home/$USERNAME/go/bin/go1.18.2 10
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
