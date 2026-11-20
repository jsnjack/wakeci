package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"slices"

	bolt "go.etcd.io/bbolt"
	yaml "gopkg.in/yaml.v2"
)

// HandleJobsView returns all available jobs
// @Summary      Returns list of available jobs
// @Description  API key scope: read; a key or a single sign-on user only sees their jobs
// @Tags         jobs
// @Produce      json
// @Param        board    query    boolean  false  "With 1, each job's last 20 builds (recent), failing streak (failing) and next scheduled run (nextRun), as the jobs page shows them"
// @Success      200      {array}    JobsListData
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      429      {string}   string
// @Failure      500      {string}   string
// @Router       /api/jobs/ [get]
func HandleJobsView(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	access := accessFromContext(r.Context())
	// [], not null, when there are no jobs
	data := []*JobsListData{}
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(JobsBucket))
		c := b.Cursor()
		for key, _ := c.First(); key != nil; key, _ = c.Next() {
			if !access.Reads(string(key)) {
				continue
			}
			job := JobsListData{
				Name: string(key),
			}
			jb := b.Bucket(key)
			if jb != nil {
				params := jb.Get([]byte("defaultParams"))
				err := json.Unmarshal(params, &job.DefaultParams)
				if err != nil {
					return err
				}
				if job.DefaultParams == nil {
					job.DefaultParams = []map[string]string{}
				}
				job.Labels = []string{}
				if labels := jb.Get([]byte("labels")); labels != nil {
					if err := json.Unmarshal(labels, &job.Labels); err != nil {
						return err
					}
				}
				job.ParamInfo = map[string]ParamInfo{}
				if info := jb.Get([]byte("paramInfo")); info != nil {
					if err := json.Unmarshal(info, &job.ParamInfo); err != nil {
						return err
					}
				}
				desc := jb.Get([]byte("desc"))
				job.Desc = string(desc)
				interval := jb.Get([]byte("interval"))
				job.Interval = string(interval)
				active := jb.Get([]byte("active"))
				job.Active = string(active)
				job.Group = string(jb.Get([]byte("group")))
			}
			data = append(data, &job)
		}
		return nil
	})
	if err == nil && r.URL.Query().Get("board") == "1" {
		err = addBoard(data)
	}
	if err != nil {
		logger.Error("list jobs", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	payloadB, err := json.Marshal(data)
	if err != nil {
		logger.Error("marshal jobs list", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeBody(logger, w, payloadB)
}

// HandleJobsCreate creates a new job file from default template
// @Summary      Create new empty job
// @Description  The job is created from the default template. API key scope: write. All parameters are available as query parameters and as formData
// @Tags         jobs
// @Produce      plain
// @Param        name     formData    string   true   "Name of the job (also the name of the file in which the job is stored)"
// @Success      200      {string}    string
// @Failure      400      {string}   string
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      429      {string}   string
// @Failure      500      {string}    string
// @Router       /api/jobs/create [post]
func HandleJobsCreate(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	name := r.FormValue("name")
	if !isValidJobName(name) {
		// Not the name in the logs: it could be a secret typed into the
		// wrong field
		addAuditAttrs(r, "job", "invalid")
		logger.Warn("invalid job name")
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte("Invalid job name"))
		return
	}
	addAuditAttrs(r, "job", name)
	path := Config.JobDir + name + Config.jobsExt

	if _, err := os.Stat(path); err == nil {
		logger.Warn("job file already exists", "path", path)
		w.WriteHeader(http.StatusBadRequest)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte("Job with this name already exists"))
		return
	} else if os.IsNotExist(err) {
		// Verify that it is still a valid yaml file and it is possible to create
		// a job out of it
		job := Job{}
		err := yaml.Unmarshal([]byte(NewJobTemplate), &job)
		if err != nil {
			logger.Error("unmarshal new job template", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain")
			writeBody(logger, w, []byte(err.Error()))
			return
		}
		err = os.WriteFile(path, []byte(NewJobTemplate), 0644)
		if err != nil {
			logger.Error("write job file", "path", path, "err", err)
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "text/plain")
			writeBody(logger, w, []byte(err.Error()))
			return
		}
		logger.Info("job created", "job", name)
		err = RegisterJob(path)
		if err != nil {
			logger.Error("register job", "job", name, "err", err)
			// The file stays, so the watcher or a restart can still add the job
			addAuditAttrs(r, "fileCreated", true)
			w.WriteHeader(http.StatusInternalServerError)
			writeBody(logger, w, []byte(err.Error()))
			return
		}
	} else {
		logger.Error("stat job file", "path", path, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
}

// boardBuilds is how many builds of each job the jobs page shows, and
// boardScan how many build records it reads at most to find them
const (
	boardBuilds = 20
	boardScan   = 20000
)

// addBoard adds what the jobs page shows of each job (J1): its last builds,
// from the build stats, newest first until every job has boardBuilds or
// boardScan records were read; its failing streak; and its next scheduled
// run
func addBoard(jobs []*JobsListData) error {
	byName := make(map[string]*JobsListData, len(jobs))
	for _, job := range jobs {
		byName[job.Name] = job
	}
	recent := make(map[string][]*buildStats, len(jobs))
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BuildStatsBucket)
		if b == nil {
			return nil
		}
		full, read := 0, 0
		c := b.Cursor()
		for k, v := c.Last(); k != nil && read < boardScan && full < len(jobs); k, v = c.Prev() {
			read++
			var s buildStats
			if json.Unmarshal(v, &s) != nil {
				continue
			}
			if _, ok := byName[s.Job]; !ok || len(recent[s.Job]) >= boardBuilds {
				continue
			}
			recent[s.Job] = append(recent[s.Job], &s)
			if len(recent[s.Job]) == boardBuilds {
				full++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for name, builds := range recent {
		slices.Reverse(builds)
		job := byName[name]
		job.Recent = make([]BuildOutcome, 0, len(builds))
		for _, s := range builds {
			job.Recent = append(job.Recent, outcomeOf(s))
		}
		job.Failing = failingStreak(builds)
	}
	for _, entry := range GlobalCron.Entries() {
		if cronJob, ok := entry.Job.(*Job); ok {
			if job, ok := byName[cronJob.Name]; ok && !entry.Next.IsZero() {
				next := entry.Next
				job.NextRun = &next
			}
		}
	}
	return nil
}
