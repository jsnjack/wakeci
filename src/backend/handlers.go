package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"

	bolt "github.com/etcd-io/bbolt"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/crypto/bcrypt"
)

// HandleRunJob adds job to queue
func HandleRunJob(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	jobFile := *WorkingDirFlag + ps.ByName("name") + ".yaml"
	job, err := ReadJob(jobFile)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	build, err := CreateBuild(job)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Update params from URL
	for idx := range build.Params {
		for pkey := range build.Params[idx] {
			value := r.URL.Query().Get(pkey)
			if value != "" {
				build.Params[idx][pkey] = value
				logger.Printf("Updating key %s to %s", pkey, value)
			}
		}
	}

	// Create workspace
	err = os.MkdirAll(build.GetWorkspaceDir(), os.ModePerm)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	logger.Printf("Workspace %s has been created\n", build.GetWorkspaceDir())

	// Create wakespace
	err = os.MkdirAll(build.GetWakespaceDir(), os.ModePerm)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	logger.Printf("Wakespace %s has been created\n", build.GetWakespaceDir())

	// Copy job config
	input, err := ioutil.ReadFile(jobFile)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = ioutil.WriteFile(build.GetBuildConfigFilename(), input, 0644)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	logger.Printf("Build config %s has been created\n", build.GetBuildConfigFilename())

	Q.Add(build)
	Q.Take()
	build.BroadcastUpdate()
	defer w.Write([]byte(strconv.Itoa(build.ID)))
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
		return
	}
	// Collect tasks info by reconstructing jon object
	buildConfigFilename := *WorkingDirFlag + "wakespace/" + strconv.Itoa(buildID) + "/build.yaml"
	if _, err := os.Stat(buildConfigFilename); os.IsNotExist(err) {
		logger.Println(err)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	job, err := ReadJob(buildConfigFilename)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
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

	pageS := r.URL.Query().Get("page")
	pageI, err := strconv.Atoi(pageS)
	if err != nil {
		logger.Printf("Invalid page %s", pageS)
		pageI = 1
		return
	}

	if pageI < 1 {
		pageI = 1
	}

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
		binary.BigEndian.PutUint64(fromB, binary.BigEndian.Uint64(lastK)-uint64((pageI-1)*pageSize))
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
				payload = append(payload, &msg)
				count++
				if count >= pageSize {
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
			}
			data = append(data, &job)
		}
		return nil
	})
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	payloadB, err := json.Marshal(data)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(payloadB)
}

// HandleReloadTaskLog broadcasts all logs from a filesystem file
func HandleReloadTaskLog(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	buildID := ps.ByName("id")
	taskID := ps.ByName("taskID")
	// Verify ids
	_, err := strconv.Atoi(buildID)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	taskIDint, err := strconv.Atoi(taskID)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	path := *WorkingDirFlag + "wakespace/" + buildID + "/" + "task_" + taskID + ".log"
	// Verify that path exists
	_, err = os.Stat(path)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	// Read file
	f, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	var counter int
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}

			logger.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		msg := MsgBroadcast{
			Type: "build:log:" + buildID,
			Data: &CommandLogData{
				TaskID: taskIDint,
				ID:     counter,
				Data:   line,
			},
		}
		counter++
		BroadcastChannel <- &msg
	}
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
			return
		}
	}

	cb := r.FormValue("concurrentBuilds")
	cbInt, err := strconv.Atoi(cb)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	Q.SetConcurrency(cbInt)
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
		return nil
	})

	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	payloadB, err := json.Marshal(settings)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(payloadB)
}
