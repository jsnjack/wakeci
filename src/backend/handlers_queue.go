package main

import (
	"net/http"
)

// HandleQueueGet returns whether queued builds start
// @Summary      Return whether queued builds start
// @Description  stopping is true while wakeci waits for the running builds before it stops, or restarts after an update; builds still running at abortAt are aborted. version is the running version. Not available with an API key
// @Tags         queue
// @Produce      json
// @Success      200      {object}   QueueStatusData
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      429      {string}   string
// @Router       /api/queue/ [get]
func HandleQueueGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, requestLogger(r), GlobalQueue.Status())
}
