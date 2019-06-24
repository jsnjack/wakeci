package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"sync"

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
const WorkingDir = "/home/jsn/workspace/wakeci/test_wd/"

// DB is the Bolt db
var DB *bolt.DB

// Q is a global queue object
var Q *Queue

func init() {
	PortFlag = flag.String("port", "8081", "Port to start the server on")
	flag.Parse()

	Logger = log.New(os.Stdout, "", log.Lmicroseconds|log.Lshortfile)
}

func main() {
	var err error
	DB, err = bolt.Open(WorkingDir+"wakeci.db", 0644, nil)
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
		_, err = tx.CreateBucketIfNotExists(GlobalBucket)
		if err != nil {
			return err
		}
		_, err = tx.CreateBucketIfNotExists(HistoryBucket)
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		Logger.Fatal(err)
	}

	Q = &Queue{
		concurrentBuilds: 2,
		mutex:            &sync.Mutex{},
	}

	ScanAllJobs()

	go func() {
		// Websocket section
		router := httprouter.New()
		router.GET("/ws", handleWSConnection)
		router.GET("/api/feed/", LogMi(CORSMi(HandleFeedView)))
		router.GET("/api/jobs/", LogMi(CORSMi(HandleJobsView)))
		router.POST("/api/job/:name/run", LogMi(CORSMi(HandleRunJob)))
		router.GET("/api/build/:id/", LogMi(CORSMi(HandleGetBuild)))
		router.POST("/api/build/:id/abort", LogMi(CORSMi(HandleAbortBuild)))
		router.GET("/api/build/:id/log/:taskID/", LogMi(CORSMi(HandleReloadTaskLog)))

		go BroadcastMessage()

		Logger.Println("Starting ws server on port " + *PortFlag)
		err := http.ListenAndServe(":"+*PortFlag, router)
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	}()

	select {}
}
