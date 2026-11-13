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

// HandleQueueAbort aborts the running builds of a stopping wakeci now
// @Summary      Abort the running builds now while wakeci stops
// @Description  While wakeci stops or restarts (stopping in GET /api/queue/), aborts the running builds now instead of at abortAt, like a second SIGTERM. Their on_aborted and finally tasks still run, for up to 2 minutes, then wakeci stops, or restarts after an update. Queued builds stay queued. 409 unless wakeci is stopping. Needs the write scope. Not available with an API key
// @Tags         queue
// @Produce      plain
// @Success      200      {string}   string
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      409      {string}   string
// @Failure      429      {string}   string
// @Router       /api/queue/abort [post]
func HandleQueueAbort(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	if err := GlobalQueue.AbortNow(abortedBy(r)); err != nil {
		writePlainText(w, logger, http.StatusConflict, "wakeci isn't stopping.")
		return
	}
	writePlainText(w, logger, http.StatusOK, "Aborting the running builds.")
}
