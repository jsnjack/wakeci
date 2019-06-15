package main

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"

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
	defer w.Write([]byte("{}"))
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

// HandleFeed returns items in current feed - executed and queued jobs
func HandleFeed(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	var payload []*BuildUpdateData
	for _, b := range BuildList {
		payload = append(payload, &BuildUpdateData{
			ID:         b.ID,
			Name:       b.Job.Name,
			Status:     b.Status,
			TotalTasks: len(b.Job.Tasks),
			DoneTasks:  b.GetNumberOfFinishedTasks(),
		})
	}
	for _, b := range BuildQueue {
		payload = append(payload, &BuildUpdateData{
			ID:         b.ID,
			Name:       b.Job.Name,
			Status:     b.Status,
			TotalTasks: len(b.Job.Tasks),
			DoneTasks:  b.GetNumberOfFinishedTasks(),
		})
	}
	payloadB, err := json.Marshal(payload)
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(payloadB)
}
