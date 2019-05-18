package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func handleJobRun(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	job, err := ReadJob(WorkingDir + "/" + ps.ByName("name") + ".yaml")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	exec := CreateExecutor(job)
	go exec.Start()
}
