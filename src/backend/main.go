package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"sync"

	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/crypto/bcrypt"

	rice "github.com/GeertJohan/go.rice"
	bolt "github.com/etcd-io/bbolt"
	"github.com/julienschmidt/httprouter"
)

// Logger is the main logger
var Logger *log.Logger

// PortFlag is a port on which the server should be started
var PortFlag *string

// HostnameFlag is the domain name for autocert. Active only when port is 443
var HostnameFlag *string

// WorkingDirFlag contains path to the working directory
var WorkingDirFlag *string

// Version is the version of the application calculated with monova
var Version string

// DB is the Bolt db
var DB *bolt.DB

// Q is a global queue object
var Q *Queue

func init() {
	PortFlag = flag.String("port", "8081", "Port to start the server on")
	HostnameFlag = flag.String("hostname", "wakeci.dev", "Hostname for autocert. Active only whem port is 443")
	WorkingDirFlag = flag.String("wd", "~/.wakeci/", "Working directory")
	flag.Parse()

	Logger = log.New(os.Stdout, "", log.Lmicroseconds|log.Lshortfile)
}

func main() {
	var err error
	err = os.MkdirAll(*WorkingDirFlag, os.ModePerm)
	if err != nil {
		Logger.Fatal(err)
	}

	DB, err = bolt.Open(*WorkingDirFlag+"wakeci.db", 0644, nil)
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

		gb, err := tx.CreateBucketIfNotExists(GlobalBucket)
		if err != nil {
			return err
		}
		password := gb.Get([]byte("password"))
		if password == nil {
			Logger.Println("Creating default password...")
			passwordH, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			err = gb.Put([]byte("password"), passwordH)
			if err != nil {
				return err
			}
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

	go BroadcastMessage()

	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache("certs"),
		HostPolicy: autocert.HostWhitelist(*HostnameFlag),
	}

	// Configure routes
	router := httprouter.New()
	router.NotFound = http.FileServer(rice.MustFindBox("../frontend/dist/").HTTPBox())
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

	if *PortFlag == "443" {
		go func() {
			Logger.Println("Listening on port 80...")
			err := http.ListenAndServe(":80", certManager.HTTPHandler(nil))
			if err != nil {
				Logger.Fatal(err)
			}
		}()

		Logger.Println("Listening on port 443...")
		server := &http.Server{
			Addr: ":443",
			TLSConfig: &tls.Config{
				GetCertificate: certManager.GetCertificate,
			},
			Handler: router,
		}

		err = server.ListenAndServeTLS("", "")
		if err != nil {
			Logger.Fatal(err)
		}
	} else {
		Logger.Printf("Listening on port %s...\n", *PortFlag)
		err := http.ListenAndServe(":"+*PortFlag, router)
		if err != nil {
			Logger.Fatal(err)
		}
	}
}
