package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/julienschmidt/httprouter"
)

// HandlerLogger is a special type for loggers per request
type HandlerLogger string

// HL is a handle logger
const HL HandlerLogger = "logger"

// LogMi is a middleware that creates a new logger per request and logs total time that took to process a request
func LogMi(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		startTime := time.Now()

		// Take the context out from the request
		ctx := r.Context()

		logID := GenerateRandomString(5)

		// Get the settings
		handlerLogger := log.New(os.Stdout, "["+logID+"] ", log.Lmicroseconds|log.Lshortfile)

		// Get new context with key-value "settings"
		ctx = context.WithValue(ctx, HL, handlerLogger)

		// Get new http.Request with the new context
		r = r.WithContext(ctx)

		// Call actuall handler
		next(w, r, ps)

		defer func() {
			duration := time.Now().Sub(startTime)
			handlerLogger.Printf("%s %s took %s\n", r.Method, r.URL, duration)
		}()
	})
}

// CORSMi adds CORS headers
func CORSMi(next httprouter.Handle) httprouter.Handle {
	return httprouter.Handle(func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		// Call actuall handler
		next(w, r, ps)
		w.Header().Set("Access-Control-Allow-Origin", "*")
	})
}
