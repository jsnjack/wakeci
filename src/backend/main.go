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
	"github.com/go-chi/chi/v5"
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

	router := chi.NewRouter()
	router.Use(LogMi)
	router.Use(SecurityMi)
	router.Use(CORSMi)

	router.With(AuthMi).Get("/ws", HandleWS)

	router.Route("/auth", func(router chi.Router) {
		router.With(AuthMi).Get("/_isLoggedIn", HandleIsLoggedIn)
		router.Post("/login", HandleLogIn)
		router.Get("/logout", HandleLogOut)
	})

	router.Route("/api", func(router chi.Router) {
		router.Use(AuthMi)
		router.Get("/feed", HandleFeedView)

		router.Route("/jobs", func(router chi.Router) {
			router.Get("/", HandleJobsView)
			router.Post("/create", HandleJobsCreate)
			router.Post("/refresh", HandleJobsRefresh)
		})

		router.Route("/job", func(router chi.Router) {
			router.Post("/{name}/run", HandleRunJob)
			router.Delete("/{name}", HandleDeleteJob)
			router.Post("/{name}", HandleJobPost)
			router.Get("/{name}", HandleJobGet)
			router.Post("/{name}/set_active", HandleJobSetActive)
		})

		router.Route("/build", func(router chi.Router) {
			router.Get("/{id}", HandleGetBuild)
			router.Post("/{id}/abort", HandleAbortBuild)
			router.Post("/{id}/flush", HandleFlushTaskLogs)
		})

		router.Get("/settings", HandleSettingsGet)
		router.Post("/settings", HandleSettingsPost)
	})

	router.Route("/internal", func(router chi.Router) {
		router.Use(InternalAuthMi)
		router.Post("/api/job/{name}/run", HandleRunJob)
	})

	router.Route("/storage", func(router chi.Router) {
		// Storage server
		router.Use(AuthMi)
		storageServer := http.FileServer(http.Dir(Config.WorkDir + "wakespace"))
		router.Method("GET", "/build/*", HandleWakespaceResource(storageServer))
		router.Method("HEAD", "/build/*", HandleWakespaceResource(storageServer))
	})

	vuefs := http.FileServer(http.FS(Assets))
	router.Method("GET", "/*", HandleVueResources(vuefs))

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
