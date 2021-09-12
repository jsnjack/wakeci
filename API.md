# API

## Authentication
Use HTTP Basic Authentication with empty username and your password:
```bash
curl -u :admin https://wake.ci/api/feed/
```

## Endpoints

### GET /api/feed/
Returns a list with 10 latest builds

#### Input (query parameters)
- _offset_ - `number` - skip _n_ latest builds
- _filter_ - `string` - returns only builds which `ID`, `name`, `params` or `status` contains provided string

#### Output
```json
[
  {
    "id": 1892,
    "name": "curious_cow",
    "status": "finished",
    "tasks": [
      {
        "id": 1,
        "status": "finished",
        "startedAt": "2020-01-02T14:26:22.483139649+01:00",
        "duration": 29305521,
        "kind": "main"
      }
    ],
    "params": [
      {
        "SLEEP": "5"
      }
    ],
    "artifacts": null,
    "startedAt": "2020-01-02T14:26:17.464528762+01:00",
    "duration": 5048203514
  }
]
```

---

### GET /api/jobs/
Returns a list of available jobs

#### Output
```json
[
  {
    "name": "curious_cow",
    "desc": "Ask a cow to say something smart",
    "defaultParams": [
      {
        "SLEEP": "5"
      }
    ],
    "interval": "@every 2h",
    "active": "true"
  },
]
```

---

### POST /api/jobs/create
Creates new empty job

#### Input (query parameters or form data)
- _name_ - `string` - name of the job (also name of the file in which the job is stored)

---

---

### POST /api/jobs/refresh
Refresh all jobs from the configuration folder; removes non-existing jobs

---

### POST /api/job/:name/run
Schedules new build for a job. Returns build id

#### Input (query parameters or form data)
`params` to overwrite default values

#### Output
```
32
```

---

### DELETE /api/job/:name/
Deletes the job

---

### POST /api/job/:name/
Updates the content of the job

#### Input (query parameters or form data)
- _fileContent_ - `string` - new content of the job

---

### GET /api/job/:name/
Returns the content of the job

#### Output
```
{
  "fileContent": "desc: Ask a cow to say something smart\r\nparams:\r\n  - SLEEP: 5\r\n\r\ntasks:\r\n  - name: Waking up a cow\r\n    run: sleep ${SLEEP}\r\n\r\n  - name: Cow says\r\n    run: fortune | cowsay\r\n\r\ninterval: \"@every 2h\"\r\nallow_parallel_builds: no\r\non_running:\r\n  - name: Running logger on running\r\n    run: logger \"Running build ${WAKE_BUILD_ID}\"\r\n\r\non_pending:\r\n  - name: Print content of job\r\n    run: cat ${WAKE_CONFIG_DIR}curious_cow.yaml"
}
```

---

### POST /api/job/:name/set_active/
Toggles job status. Returns new status of the job

#### Input (query parameters or form data)
- _active_ - `string` - true or false

#### Output
```
true
```

---

### GET /api/build/:id/
Returns status of the build

#### Output
```json
{
  "job": {
    "name": "build",
    "desc": "Ask a cow to say something smart",
    "tasks": [
      {
        "id": 0,
        "name": "Print content of job",
        "run": "cat ${WAKE_CONFIG_DIR}curious_cow.yaml",
        "status": "pending",
        "kind": "pending",
        "logs": null
      },
      {
        "id": 1,
        "name": "Running logger on running",
        "run": "logger \"Running build ${WAKE_BUILD_ID}\"",
        "status": "pending",
        "kind": "running",
        "logs": null
      },
      {
        "id": 2,
        "name": "Waking up a cow",
        "run": "sleep ${SLEEP}",
        "status": "pending",
        "kind": "main",
        "logs": null
      },
      {
        "id": 3,
        "name": "Cow says",
        "run": "fortune | cowsay",
        "status": "pending",
        "kind": "main",
        "logs": null
      }
    ],
    "defaultParams": [
      {
        "SLEEP": "5"
      }
    ],
    "artifacts": null,
    "interval": "@every 2h",
    "timeout": "",
    "AllowParallel": false
  },
  "status_update": {
    "id": 1911,
    "name": "curious_cow",
    "status": "finished",
    "tasks": [
      {
        "id": 0,
        "status": "finished",
        "startedAt": "2020-01-08T23:21:24.638692997+01:00",
        "duration": 14281683,
        "kind": "pending"
      },
      {
        "id": 1,
        "status": "finished",
        "startedAt": "2020-01-08T23:21:24.652985547+01:00",
        "duration": 7483607,
        "kind": "running"
      },
      {
        "id": 2,
        "status": "finished",
        "startedAt": "2020-01-08T23:21:24.661016746+01:00",
        "duration": 5006031419,
        "kind": "main"
      },
      {
        "id": 3,
        "status": "finished",
        "startedAt": "2020-01-08T23:21:29.668957082+01:00",
        "duration": 58101612,
        "kind": "main"
      }
    ],
    "params": [
      {
        "SLEEP": "5"
      }
    ],
    "artifacts": null,
    "startedAt": "2020-01-08T23:21:24.65298512+01:00",
    "duration": 5074441551
  }
}
```

---

### POST /api/build/:id/abort
Aborts the build

---

### GET /api/settings/
Returns application settings

#### Output
```json
{
  "concurrentBuilds": 6,
  "buildHistorySize": 200
}

```

---

### POST /api/settings/
Updates application settings

#### Input (query parameters or form data)
- _password_ - `string`
- _concurrentBuilds_ - `number`
- _buildHistorySize_ - `number`

---


## Internal endpoints
Internal endpoints are allowed to be called only from localhost. They do not require credentials

### POST /api/job/:name/run
Schedules new build for a job. Returns build id

#### Input (query parameters or form data)
`params` to overwrite default values

#### Output
```
32
```
