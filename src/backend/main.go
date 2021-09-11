package main

import (
	"crypto/tls"
	"embed"
	"flag"
	"log"
	"net/http"
	"os"

	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/crypto/bcrypt"

	"github.com/NYTimes/gziphandler"
	"github.com/julienschmidt/httprouter"
	"github.com/robfig/cron/v3"
	bolt "go.etcd.io/bbolt"
)

// Logger is the main logger
var Logger *log.Logger

// Version is the version of the application calculated with monova
var Version string

// DB is the Bolt db
var DB *bolt.DB

// GlobalQueue is a global queue object
var GlobalQueue *Queue

// GlobalCron is a global cron object
var GlobalCron *cron.Cron

// GlobalSessionStorage is a global session storage object
var GlobalSessionStorage *SessionStorage

// Config is a global configuration object
var Config *WakeConfig

// WSHub is the websocket hub
var WSHub *Hub

//go:embed assets/*
var Assets embed.FS

func init() {
	Logger = log.New(os.Stdout, "", log.Lmicroseconds|log.Lshortfile)

	configFlag := flag.String("config", "Wakefile.yaml", "Configuration file location")
	flag.Parse()

	var err error
	Config, err = CreateWakeConfig(*configFlag)
	if err != nil {
		Logger.Fatal(err)
	}
}

func main() {
	var err error
	err = os.MkdirAll(Config.WorkDir, os.ModePerm)
	if err != nil {
		Logger.Fatal(err)
	}

	DB, err = bolt.Open(Config.WorkDir+"wakeci.db", 0644, nil)
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
			err = gb.Put([]byte("concurrentBuilds"), IntToByte(2))
			if err != nil {
				return err
			}
			err = gb.Put([]byte("buildHistorySize"), IntToByte(200))
			if err != nil {
				return err
			}
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

	GlobalSessionStorage = CreateSessionStorage(SessionCleanupPeriod)

	GlobalQueue, err = CreateQueue()
	if err != nil {
		Logger.Fatal(err)
	}

	GlobalCron = cron.New()
	GlobalCron.Start()

	CleanupJobsBucket()
	ScanAllJobs()
	CleanupOldBuilds(BuildCleanupPeriod)

	WSHub = newHub()
	go WSHub.run()

	certManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache("certs"),
		HostPolicy: autocert.HostWhitelist(Config.Hostname),
	}

	vuefs := http.FileServer(http.FS(Assets))
	storageServer := http.FileServer(http.Dir(Config.WorkDir + "wakespace"))
	// Configure routes
	router := httprouter.New()
	// Assume that all unknown routes are vue-related files
	router.NotFound = VueResourcesMi(vuefs)

	// Types of endpoints
	privateEndpoints := MiddlewareChain{}
	privateEndpoints.Add(LogMi, CORSMi, AuthMi)

	privateStorageEndpoints := MiddlewareChain{}
	privateStorageEndpoints.Add(LogMi, AuthMi)

	internalEndpoints := MiddlewareChain{}
	internalEndpoints.Add(LogMi, InternalAuthMi)

	publicEndpoints := MiddlewareChain{}
	publicEndpoints.Add(LogMi, CORSMi)

	// For artifacts
	router.GET("/storage/build/*filepath", privateStorageEndpoints.Handle(WakespaceResourceMi(storageServer)))
	router.HEAD("/storage/build/*filepath", privateStorageEndpoints.Handle(WakespaceResourceMi(storageServer)))

	// Websocket section
	router.GET("/ws", AuthMi(HandleWS))

	// Auth urls
	router.GET("/auth/_isLoggedIn/", privateEndpoints.Handle(HandleIsLoggedIn))
	router.POST("/auth/login/", publicEndpoints.Handle(HandleLogIn))
	router.GET("/auth/logout/", publicEndpoints.Handle(HandleLogOut))

	// API calls used by client application
	router.GET("/api/feed/", privateEndpoints.Handle(HandleFeedView))

	router.GET("/api/jobs/", privateEndpoints.Handle(HandleJobsView))
	router.POST("/api/jobs/create", privateEndpoints.Handle(HandleJobsCreate))
	router.POST("/api/jobs/refresh", privateEndpoints.Handle(HandleJobsRefresh))
	router.POST("/api/job/:name/run", privateEndpoints.Handle(HandleRunJob))
	router.DELETE("/api/job/:name/", privateEndpoints.Handle(HandleDeleteJob))
	router.POST("/api/job/:name/", privateEndpoints.Handle(HandleJobPost))
	router.GET("/api/job/:name/", privateEndpoints.Handle(HandleJobGet))
	router.POST("/api/job/:name/set_active/", privateEndpoints.Handle(HandleJobSetActive))

	router.GET("/api/build/:id/", privateEndpoints.Handle(HandleGetBuild))
	router.POST("/api/build/:id/abort", privateEndpoints.Handle(HandleAbortBuild))
	router.POST("/api/build/:id/flush", privateEndpoints.Handle(HandleFlushTaskLogs))

	router.POST("/api/settings/", privateEndpoints.Handle(HandleSettingsPost))
	router.GET("/api/settings/", privateEndpoints.Handle(HandleSettingsGet))

	// Internal API
	router.POST("/internal/api/job/:name/run", internalEndpoints.Handle(HandleRunJob))

	if Config.Port == "443" {
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
				// https://ssl-config.mozilla.org/#server=golang&version=1.13.6&config=intermediate&guideline=5.4
				MinVersion:               tls.VersionTLS12,
				PreferServerCipherSuites: false,
				CipherSuites: []uint16{
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				},
				GetCertificate: certManager.GetCertificate,
			},
			Handler: gziphandler.GzipHandler(router),
		}

		err = server.ListenAndServeTLS("", "")
		if err != nil {
			Logger.Fatal(err)
		}
	} else {
		Logger.Printf("Listening on port %s...\n", Config.Port)
		err := http.ListenAndServe(":"+Config.Port, gziphandler.GzipHandler(router))
		if err != nil {
			Logger.Fatal(err)
		}
	}
}
