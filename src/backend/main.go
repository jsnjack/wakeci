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
		_, err = tx.CreateBucketIfNotExists(SessionBucket)
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
		router := httprouter.New()
		// Websocket section
		router.GET("/ws", AuthMi(handleWSConnection))

		// Auth urls
		router.GET("/auth/_isLoggedIn/", LogMi(CORSMi(AuthMi(HandleIsLoggedIn))))
		router.POST("/auth/login/", LogMi(CORSMi(HandleLogIn)))
		router.GET("/auth/logout/", LogMi(CORSMi(HandleLogOut)))

		// API calls used by client application
		router.GET("/api/feed/", LogMi(CORSMi(AuthMi(HandleFeedView))))
		router.GET("/api/jobs/", LogMi(CORSMi(AuthMi(HandleJobsView))))
		router.POST("/api/job/:name/run", LogMi(CORSMi(AuthMi(HandleRunJob))))
		router.GET("/api/build/:id/", LogMi(CORSMi(AuthMi(HandleGetBuild))))
		router.POST("/api/build/:id/abort", LogMi(CORSMi(AuthMi(HandleAbortBuild))))
		router.GET("/api/build/:id/log/:taskID/", LogMi(CORSMi(AuthMi(HandleReloadTaskLog))))

		go BroadcastMessage()

		Logger.Println("Starting ws server on port " + *PortFlag)
		err := http.ListenAndServe(":"+*PortFlag, router)
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	}()

	select {}
}
