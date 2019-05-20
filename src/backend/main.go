package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	bolt "github.com/etcd-io/bbolt"
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

// DB is the Bolt db
var DB *bolt.DB

func init() {
	PortFlag = flag.String("port", "8081", "Port to start the server on")
	flag.Parse()

	Logger = log.New(os.Stdout, "", log.Lmicroseconds|log.Lshortfile)
}

func main() {
	var err error
	DB, err = bolt.Open(WorkingDir+"npci.db", 0644, nil)
	if err != nil {
		Logger.Fatal(err)
	}
	defer DB.Close()

	// Bootstrap DB
	err = DB.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(JobsBucket)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		Logger.Fatal(err)
	}

	ScanAllJobs()

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
