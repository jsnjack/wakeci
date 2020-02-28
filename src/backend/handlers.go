package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	bolt "github.com/etcd-io/bbolt"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/crypto/bcrypt"
	yaml "gopkg.in/yaml.v2"
)

// HandleRunJob adds job to queue
func HandleRunJob(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	err := r.ParseForm()
	if err != nil {
		logger.Println(err)
	}

	build, err := RunJob(ps.ByName("name"), r.Form)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte(strconv.Itoa(build.ID)))
}

// HandleGetBuild Returns information required to bootstrap build page
func HandleGetBuild(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	idp := ps.ByName("id")
	buildID, err := strconv.Atoi(idp)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	// Collect tasks info by reconstructing job object
	buildConfigFilename := Config.WorkDir + "wakespace/" + strconv.Itoa(buildID) + "/build" + Config.jobsExt
	if _, err := os.Stat(buildConfigFilename); os.IsNotExist(err) {
		logger.Println(err)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(err.Error()))
		return
	}

	job, err := CreateJobFromFile(buildConfigFilename)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	// Get build statusupdate
	var buildStatusData BuildUpdateData
	err = DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(HistoryBucket))
		ud := b.Get(Itob(buildID))
		if ud == nil {
			return fmt.Errorf("Not found")
		}
		return json.Unmarshal(ud, &buildStatusData)
	})
	if err != nil {
		logger.Println(err)
		if err.Error() == "Not found" {
			w.WriteHeader(http.StatusNotFound)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		}
		return
	}
	payload := struct {
		Job          *Job             `json:"job"`
		StatusUpdate *BuildUpdateData `json:"status_update"`
	}{
		Job:          job,
		StatusUpdate: &buildStatusData,
	}

	payloadB, err := json.Marshal(payload)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(payloadB)
}

// HandleFeedView returns items in current feed - executed and queued jobs
func HandleFeedView(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	const pageSize = 10

	offsetS := r.URL.Query().Get("offset")
	offset, err := strconv.Atoi(offsetS)
	if err != nil {
		logger.Printf("Invalid offset %s", offsetS)
		return
	}

	if offset < 0 {
		offset = 1
	}

	filter := r.URL.Query().Get("filter")

	var payload []*BuildUpdateData
	err = DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(HistoryBucket))
		c := b.Cursor()
		count := 0
		// Check what is the last one
		lastK, _ := c.Last()
		if lastK == nil {
			return nil
		}
		// Find starting point
		fromB := make([]byte, 8)
		if filter == "" {
			binary.BigEndian.PutUint64(fromB, binary.BigEndian.Uint64(lastK)-uint64(offset))
		} else {
			// If interval is specified, always iterate from the beginning to take
			// into account offset later
			fromB = lastK
		}
		for key, v := c.Seek(fromB); key != nil; key, v = c.Prev() {
			var msg BuildUpdateData
			err := json.Unmarshal(v, &msg)
			if err != nil {
				logger.Println(err)
			} else {
				switch msg.Status {
				case StatusPending, StatusRunning:
					if !Q.Verify(msg.ID) {
						msg.Status = StatusAborted
						updatedB, err := json.Marshal(msg)
						if err != nil {
							logger.Println(err)
						}
						b.Put(Itob(msg.ID), updatedB)
					}
					break
				}
				if filter != "" {
					if strings.Contains(fmt.Sprintf("%v %v %v %v", msg.ID, msg.Name, msg.Status, msg.Params), filter) {
						count++
						if count <= offset {
							continue
						}
					} else {
						continue
					}
				}
				payload = append(payload, &msg)
				if len(payload) >= pageSize {
					break
				}
			}
		}
		return nil
	})
	payloadB, err := json.Marshal(payload)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(payloadB)
}

// HandleJobsView returns all available jobs
func HandleJobsView(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	var data []*JobsListData
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(JobsBucket))
		c := b.Cursor()
		for key, _ := c.First(); key != nil; key, _ = c.Next() {
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
				desc := jb.Get([]byte("desc"))
				job.Desc = string(desc)
				interval := jb.Get([]byte("interval"))
				job.Interval = string(interval)
				active := jb.Get([]byte("active"))
				job.Active = string(active)
			}
			data = append(data, &job)
		}
		return nil
	})
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	payloadB, err := json.Marshal(data)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(payloadB)
}

// HandleAbortBuild aborts build
func HandleAbortBuild(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}
	buildID := ps.ByName("id")
	id, err := strconv.Atoi(buildID)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	err = Q.Abort(id)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
}

// HandleSettingsPost saves settings
func HandleSettingsPost(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	// Password
	password := r.FormValue("password")
	if password != "" {
		err := DB.Update(func(tx *bolt.Tx) error {
			passwordH, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				return err
			}

			gb := tx.Bucket(GlobalBucket)
			err = gb.Put([]byte("password"), passwordH)
			if err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
	}

	// Number of concurrent builds
	cb := r.FormValue("concurrentBuilds")
	cbInt, err := strconv.Atoi(cb)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	Q.SetConcurrency(cbInt)

	// Build history size
	bhs := r.FormValue("buildHistorySize")
	bhsInt, err := strconv.Atoi(bhs)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	err = DB.Update(func(tx *bolt.Tx) error {
		gb := tx.Bucket(GlobalBucket)
		err = gb.Put([]byte("buildHistorySize"), IntToByte(bhsInt))
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
}

// HandleSettingsGet retrieves settings
func HandleSettingsGet(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}
	var settings SettingsData

	err := DB.View(func(tx *bolt.Tx) error {
		gb := tx.Bucket(GlobalBucket)
		cb, err := ByteToInt(gb.Get([]byte("concurrentBuilds")))
		if err != nil {
			return err
		}
		settings.ConcurrentBuilds = cb

		bhs, err := ByteToInt(gb.Get([]byte("buildHistorySize")))
		if err != nil {
			return err
		}
		settings.BuildHistorySize = bhs
		return nil
	})

	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	payloadB, err := json.Marshal(settings)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(payloadB)
}

// HandleJobGet returns content of a specific job file
func HandleJobGet(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	path := Config.JobDir + ps.ByName("name") + Config.jobsExt
	data, err := ioutil.ReadFile(path)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	jd := JobData{
		Content: string(data),
	}
	payloadB, err := json.Marshal(jd)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write(payloadB)
}

// HandleJobPost updates content of a specific job
func HandleJobPost(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	content := r.FormValue("fileContent")
	contentB := []byte(content)

	// Verify that it is still a valid yaml file and it is possible to create
	// a job out of it
	job := Job{}
	err := yaml.Unmarshal(contentB, &job)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	// Verify provided interval
	err = job.verifyInterval()
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}

	contentB = NormalizeNewlines(contentB)

	path := Config.JobDir + ps.ByName("name") + Config.jobsExt

	err = ioutil.WriteFile(path, contentB, 0644)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(err.Error()))
		return
	}
	logger.Printf("Job %s was updated\n", ps.ByName("name"))
	err = ScanAllJobs()
	if err != nil {
		logger.Println(err)
	}
}

// HandleJobsCreate creates a new job file from default template
func HandleJobsCreate(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	name := r.FormValue("name")
	path := Config.JobDir + name + Config.jobsExt

	if _, err := os.Stat(path); err == nil {
		logger.Printf("File %s already exists\n", path)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Job with this name already exists"))
		return
	} else if os.IsNotExist(err) {
		// Verify that it is still a valid yaml file and it is possible to create
		// a job out of it
		job := Job{}
		err := yaml.Unmarshal([]byte(NewJobTemplate), &job)
		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		err = ioutil.WriteFile(path, []byte(NewJobTemplate), 0644)
		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(err.Error()))
			return
		}
		err = ScanAllJobs()
		if err != nil {
			logger.Println(err)
		}
	} else {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
}

// HandleDeleteJob deletes the job
func HandleDeleteJob(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	name := ps.ByName("name")
	path := Config.JobDir + name + Config.jobsExt

	if _, err := os.Stat(path); err == nil {
		err = os.Remove(path)
		logger.Printf("Job %s was deleted\n", name)
		CleanupJobsBucket()
		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		return
	} else if os.IsNotExist(err) {
		logger.Println(err)
		w.WriteHeader(http.StatusNotFound)
		return
	} else {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
}

// HandleJobSetActive sets if job is active (enabled/disabled)
func HandleJobSetActive(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	name := ps.ByName("name")

	activeStatus := r.FormValue("active")

	switch activeStatus {
	case "false", "true":
		break
	default:
		m := fmt.Sprintf("Invalid active flag for a job: %s\n", activeStatus)
		logger.Printf("Invalid active flag for a job: %s\n", activeStatus)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(m))
		return
	}

	err := DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(JobsBucket))
		jb := b.Bucket([]byte(name))
		if jb == nil {
			return fmt.Errorf("Invalid job name: %s", name)
		}
		err := jb.Put([]byte("active"), []byte(activeStatus))
		if err == nil {
			logger.Printf("Change active state of job %s to %s\n", name, activeStatus)
		}
		return err
	})
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Write([]byte(activeStatus))
}

// HandleFlushTaskLogs signals to flush logs
func HandleFlushTaskLogs(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	buildID := ps.ByName("id")
	id, err := strconv.Atoi(buildID)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	err = Q.FlushLogs(id)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
}
