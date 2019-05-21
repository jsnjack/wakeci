package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

// HandleRunJob adds job to queue
func HandleRunJob(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	job, err := ReadJob(WorkingDir + "/" + ps.ByName("name") + ".yaml")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	exec, err := CreateExecutor(job)
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	Logger.Printf("New job queued: %s %d\n", exec.Job.Name, exec.ID)
	FeedQueue = append(FeedQueue, exec)
	exec.BroadcastUpdate()
	w.Write([]byte("{}"))
}
