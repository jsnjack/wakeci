package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	bolt "github.com/etcd-io/bbolt"
	"github.com/julienschmidt/httprouter"
)

// HandleRunJob adds job to queue
func HandleRunJob(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	jobFile := WorkingDir + ps.ByName("name") + ".yaml"
	job, err := ReadJob(jobFile)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	build, err := CreateBuild(job)
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Create workspace
	err = os.MkdirAll(build.GetWorkspaceDir(), os.ModePerm)
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	Logger.Printf("Workspace %s has been created\n", build.GetWorkspaceDir())

	// Create wakespace
	err = os.MkdirAll(build.GetWakespaceDir(), os.ModePerm)
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	Logger.Printf("Wakespace %s has been created\n", build.GetWakespaceDir())

	// Copy job config
	input, err := ioutil.ReadFile(jobFile)
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = ioutil.WriteFile(build.GetBuildConfigFilename(), input, 0644)
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	Logger.Printf("Build config %s has been created\n", build.GetBuildConfigFilename())

	Logger.Printf("New job queued: %s %d\n", build.Job.Name, build.ID)
	BuildQueue = append(BuildQueue, build)
	TakeFromQueue()
	build.BroadcastUpdate()
	defer w.Write([]byte(strconv.Itoa(build.ID)))
}

// HandleGetBuildInfo Returns information required to bootstrap build page
func HandleGetBuildInfo(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	buildID := ps.ByName("id")
	buildConfigFilename := WorkingDir + "wakespace/" + buildID + "/build.yaml"
	if _, err := os.Stat(buildConfigFilename); os.IsNotExist(err) {
		Logger.Println(err)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("{}"))
		return
	}

	job, err := ReadJob(buildConfigFilename)
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("{}"))
		return
	}

	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	msg := MsgBroadcast{
		Type: "build:info",
		Data: job,
	}

	msgB, err := json.Marshal(msg)
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(msgB)
}

// HandleFeedView returns items in current feed - executed and queued jobs
func HandleFeedView(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	const pageSize = 10
	var payload []*BuildUpdateData
	for _, b := range BuildList {
		payload = append(payload, b.GenerateBuildUpdateData())
	}
	for _, b := range BuildQueue {
		payload = append(payload, b.GenerateBuildUpdateData())
	}
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(HistoryBucket))
		c := b.Cursor()
		count := 0
		for key, _ := c.Last(); key != nil; key, _ = c.Prev() {
			var msg BuildUpdateData
			err := json.Unmarshal(b.Get(key), &msg)
			if err != nil {
				Logger.Println(err)
			} else {
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
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(payloadB)
}

// HandleJobsView returns items in current feed - executed and queued jobs
func HandleJobsView(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var data []*JobsListData
	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(JobsBucket))
		c := b.Cursor()
		for key, _ := c.First(); key != nil; key, _ = c.Next() {
			job := JobsListData{
				Name: string(key),
			}
			data = append(data, &job)
		}
		return nil
	})
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	payloadB, err := json.Marshal(data)
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(payloadB)
}
