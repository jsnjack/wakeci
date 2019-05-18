package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func handleJobRun(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	Logger.Println(ps)
}
