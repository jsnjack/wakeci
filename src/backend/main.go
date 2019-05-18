package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/julienschmidt/httprouter"
)

// Logger is the main logger
var Logger *log.Logger

// PortFlag is a port on which the server should be started
var PortFlag *string

// Version is the version of the application calculated with monova
var Version string

// WorkingDir is a working directory which contains all jobs
const WorkingDir = "/home/jsn/workspace/npci/test_wd/"

func init() {
	PortFlag = flag.String("port", "8081", "Port to start the server on")
	flag.Parse()

	Logger = log.New(os.Stdout, "", log.Lmicroseconds|log.Lshortfile)
}

func main() {
	go func() {
		// Websocket section
		router := httprouter.New()
		router.GET("/ws", handleWSConnection)
		router.POST("/api/job/:name/run", LogMi(CORSMi(handleJobRun)))

		go BroadcastMessages()

		Logger.Println("Starting ws server on port " + *PortFlag)
		err := http.ListenAndServe(":"+*PortFlag, router)
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	}()

	select {}
}
