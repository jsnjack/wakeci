wakeci
======

### What is it?
wakeci is an automation tool which helps to execute repetitive tasks

![ScreenShot](https://raw.githubusercontent.com/jsnjack/wakeci/master/screenshots/feed_view.png)

### Features
 - simple job configuration using YAML
 - easy to install - just download a binary file from Releases
 - automatic Let's Encrypt SSL certificates
 - parameterized builds, artifacts, intervals and timeouts - see job configuration example below
 - no plugins, no extensive configuration - focus on your project instead!

### Job configuration example
```yaml
desc: Build and release wake application
params:
  - VERSION: master

tasks:
  - name: Clone repository
    command: git clone git@github.com:jsnjack/wakeci.git --recursive

  - name: Checkout version
    command: sh ${WAKE_CONFIG_DIR}utils/checkout.sh wakeci ${VERSION}

  - name: Install npm dependencies
    command: cd wakeci/src/frontend && npm install

  - name: Build application
    command: cd wakeci && make build

  - name: Create a release on github
    command: python ${WAKE_CONFIG_DIR}utils/release_on_github.py -f wakeci/bin/wakeci -r jsnjack/wakeci -t "v`cd wakeci && monova`"

timeout: 10m

on_failed:
  - name: Send notification to Slack
    command: >-
      python ${WAKE_CONFIG_DIR}utils/notify_slack.py
      -t "Job ${WAKE_JOB_NAME} has failed <${WAKE_URL}build/${WAKE_BUILD_ID}|#${WAKE_BUILD_ID}>"
      -k error

on_finished:
  - name: Send notification to Slack
    command: >-
      python ${WAKE_CONFIG_DIR}utils/notify_slack.py
      -t "New wake version `cd wakeci && monova` <${WAKE_URL}build/${WAKE_BUILD_ID}|#${WAKE_BUILD_ID}>"
      -k ok
```
See full description [here](https://github.com/jsnjack/wakeci/blob/master/src/frontend/src/assets/configDescription.yaml)

### How to use it?
```
Usage of ./bin/wakeci:
  -config string
    	Configuration file location (default "Wakefile.yaml")
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
```

> Default password is `admin`. Don't forget to immediately change it!

### API documentation
See full description [here](https://github.com/jsnjack/wakeci/blob/master/API.md)

