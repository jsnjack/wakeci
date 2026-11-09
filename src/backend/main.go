package main

import (
	"context"
	"crypto/tls"
	"embed"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/acme/autocert"
	"golang.org/x/crypto/bcrypt"

	"github.com/CAFxX/httpcompression"
	"github.com/go-chi/chi/v5"
	"github.com/robfig/cron/v3"
	bolt "go.etcd.io/bbolt"
)

// TraceLogPath is where --trace writes its log, truncated on every start.
const TraceLogPath = "/tmp/wakeci.log"

const serverShutdownTimeout = 10 * time.Second

// Version is the version of the application calculated with monova. It is
// injected at build time via -ldflags="-X main.Version=...". The default value
// is used when the binary is built without setting it (e.g. `go build`).
var Version = "dev"

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

//go:embed docs/swagger.json
var APIDocs embed.FS

func serveUntilShutdown(
	ctx context.Context,
	serve func() error,
	shutdown func(context.Context) error,
) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- serve()
	}()

	select {
	case err := <-serveErr:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTimeout)
		defer cancel()
		if err := shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shut down server: %w", err)
		}
		if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve during shutdown: %w", err)
		}
		return nil
	}
}

func initApp() func() {
	configFlag := "Wakefile.yaml"
	flag.StringVar(&configFlag, "config", "Wakefile.yaml", "Configuration file location")
	flag.StringVar(&configFlag, "c", "Wakefile.yaml", "Configuration file location (shorthand)")
	compactDBFlag := flag.Bool("compactdb", false, "Reclaim space in the database which is no longer used")
	versionFlag := false
	flag.BoolVar(&versionFlag, "version", false, "Print the version and exit.")
	flag.BoolVar(&versionFlag, "V", false, "Print the version and exit (shorthand).")
	debug := false
	flag.BoolVar(&debug, "debug", false, "Debug-level logging on stderr.")
	flag.BoolVar(&debug, "d", false, "Debug-level logging on stderr (shorthand).")
	trace := flag.Bool("trace", false, "Trace-level logs to "+TraceLogPath+" (truncated each run).")
	flag.Parse()

	if versionFlag {
		fmt.Printf("wakeci %s\n", Version)
		os.Exit(0)
	}

	level, tracePath := "", ""
	switch {
	case *trace:
		level, tracePath = "trace", TraceLogPath
	case debug:
		level = "debug"
	}
	cleanup := initLogger(tracePath, level)

	var err error
	Config, err = CreateWakeConfig(configFlag)
	if err != nil {
		L.Error("load config", "err", err)
		cleanup()
		os.Exit(1)
	}
	initAuditLogger(os.Stderr, auditServerName(Config.Hostname))

	if *compactDBFlag {
		err = CompactDB()
		if err != nil {
			L.Error("compact db", "err", err)
			cleanup()
			os.Exit(1)
		}
		cleanup()
		os.Exit(0)
	}

	return cleanup
}

// @title wakeci API documentation
// @version 1.0

// @BasePath /api
func main() {
	cleanup := initApp()
	run(cleanup)
	// Read once run returned. Only a stop signal cancels it on the way out
	// (Queue.noteStopSignal), not, say, a request that outlasts the HTTP
	// shutdown
	if GlobalQueue.Status().RestartVersion != "" {
		// Keeps the PID, so a service manager sees the same process
		L.Warn("restarting", "binary", executablePath)
		err := syscall.Exec(executablePath, os.Args, os.Environ())
		L.Error("restart", "err", err)
		cleanup()
		os.Exit(1)
	}
	cleanup()
}

// run starts the server and returns once it stopped. wakeci starts again if
// an update planned it (Queue.BeginStop)
func run(cleanup func()) {
	fatal := func(msg string, err error) {
		L.Error(msg, "err", err)
		cleanup()
		os.Exit(1)
	}

	var err error
	executablePath, err = resolveExecutable()
	if err != nil {
		fatal("find the running binary", err)
	}

	err = os.MkdirAll(Config.WorkDir, os.ModePerm)
	if err != nil {
		fatal("create work dir", err)
	}

	DB, err = bolt.Open(Config.WorkDir+"wakeci.db", 0644, nil)
	if err != nil {
		fatal("open db", err)
	}
	defer func() {
		if err := DB.Close(); err != nil {
			L.Error("close db", "err", err)
		}
	}()

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
			L.Info("creating default password")
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

		_, err = tx.CreateBucketIfNotExists(APIKeysBucket)
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists(QueueBucket)
		if err != nil {
			return err
		}

		_, err = tx.CreateBucketIfNotExists(SessionsBucket)
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		fatal("bootstrap db", err)
	}

	GlobalSessionStorage, err = CreateSessionStorage(SessionCleanupPeriod, DB)
	if err != nil {
		fatal("load sessions", err)
	}

	if Config.SSO.Enabled() {
		GlobalSSOAuth, err = NewSSOAuth(Config.SSO)
		if err != nil {
			fatal("set up sso", err)
		}
		L.Info("sso enabled", "groups", Config.SSO.allowedGroups)
	}

	GlobalQueue, err = CreateQueue()
	if err != nil {
		fatal("create queue", err)
	}

	// Builds broadcast their updates, also the restored ones
	WSHub = newHub()
	go WSHub.run()
	GlobalSessionStorage.OnEnd = WSHub.CloseSessions

	// Runs after the cron jobs ended, so builds they queued are saved with
	// their on_pending tasks done
	defer func() {
		if !GlobalQueue.waitQueuedHooks(stopPendingTasksTimeout) {
			L.Warn("stopping before the on_pending tasks of queued builds ended")
		}
		// Before the database closes, so the builds it ends are saved. After
		// an update, a task left running would run beside the restored builds
		if !GlobalQueue.stopTasks(stopNote, stopTasksTimeout) {
			L.Warn("stopping while tasks still run; they may outlive wakeci")
		}
	}()

	// Started once the port is bound; jobs add their schedules before that
	GlobalCron = cron.New()
	defer func() {
		<-GlobalCron.Stop().Done()
	}()

	err = os.MkdirAll(Config.JobDir, os.ModePerm)
	if err != nil {
		fatal("create job dir", err)
	}

	go InitJobWatcher(Config.JobDir, Config.jobsExt)

	CleanupJobsBucket()
	ScanAllJobs()

	err = RestoreQueue(GlobalQueue)
	if err != nil {
		fatal("restore queue", err)
	}
	err = MarkInterruptedBuilds()
	if err != nil {
		L.Error("mark interrupted builds", "err", err)
	}

	CleanupOldBuilds(BuildCleanupPeriod)

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

	router.Route(authRoutePrefix, func(router chi.Router) {
		router.With(AuthMi).Get("/_isLoggedIn", HandleIsLoggedIn)
		router.Get("/methods", HandleAuthMethods)
		router.Post("/login", HandleLogIn)
		router.Get("/logout", HandleLogOut)
		router.Get(ssoLoginRoute, HandleSSOLogIn)
		router.Post(ssoACSRoute, HandleSSOACS)
	})

	router.Route("/api", apiRoutes)

	storage := HandleWakespaceResource(http.FileServer(http.Dir(Config.WorkDir + "wakespace")))
	router.Route("/storage", func(router chi.Router) {
		// Storage server
		router.Use(StorageSecurityMi)
		router.Use(AuthMi)
		router.Method("GET", "/build/*", storage)
		router.Method("HEAD", "/build/*", storage)
	})

	router.Route("/docs", func(router chi.Router) {
		router.Use(StorageSecurityMi)
		router.Get("/api/", HandleAPIDocsView)
		router.Method("GET", "/swagger.json", http.FileServer(http.FS(APIDocs)))
	})

	vuefs := http.FileServer(http.FS(Assets))
	router.Method("GET", "/*", HandleVueResources(vuefs))

	compress, err := httpcompression.DefaultAdapter()
	if err != nil {
		fatal("create compression adapter", err)
	}

	handler := compress(routeAPIKeys(newAPIKeyRouter(storage), router))
	server := &http.Server{
		Addr:    ":" + Config.Port,
		Handler: handler,
	}

	// A signal or an update stops the server once the running builds ended
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer func() {
		// From here a signal ends the process, which then doesn't restart
		signal.Stop(signals)
		close(signals)
	}()
	interrupt := forwardStopSignals(GlobalQueue, signals)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go func() {
		select {
		case sig := <-interrupt:
			L.Warn("stopping", "signal", sig)
			GlobalQueue.BeginStop(time.Now().Add(Config.drainTimeout), "")
		case <-GlobalQueue.Stopping():
		}
		drainQueue(GlobalQueue, interrupt, stopHookTimeout)
		stop()
	}()

	// Builds start only once the port is bound, so a failure to listen
	// can't cut them short
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		fatal("listen", err)
	}
	var auxiliaryServers sync.WaitGroup
	serveName := "http"
	serve := func() error { return server.Serve(listener) }

	if Config.Port == "443" {
		redirectServer := &http.Server{
			Addr:    ":80",
			Handler: certManager.HTTPHandler(nil),
		}
		auxiliaryServers.Add(1)
		go func() {
			defer auxiliaryServers.Done()
			L.Warn("Listening on :80")
			if err := serveUntilShutdown(ctx, redirectServer.ListenAndServe, redirectServer.Shutdown); err != nil {
				L.Error("port 80 redirect listener stopped", "err", err)
			}
		}()

		L.Warn("Listening on :443")
		server.TLSConfig = &tls.Config{
			// https://ssl-config.mozilla.org/#server=golang&version=1.13.6&config=intermediate&guideline=5.4
			MinVersion: tls.VersionTLS12,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			},
			GetCertificate: certManager.GetCertificate,
		}
		serveName = "tls"
		serve = func() error { return server.ServeTLS(listener, "", "") }
	} else {
		L.Warn("Listening on :" + Config.Port)
	}

	// A scheduled build would start queued ones too
	GlobalCron.Start()
	GlobalQueue.Take()
	err = serveUntilShutdown(ctx, serve, server.Shutdown)
	stop()
	auxiliaryServers.Wait()
	if err != nil {
		L.Error("serve "+serveName, "err", err)
	}
}

// apiRoutes registers the /api routes for logged-in users and the admin
// password. API keys use newAPIKeyRouter instead.
func apiRoutes(router chi.Router) {
	// Before AuthMi, so refused changes are audited too
	router.Use(AuditMi)
	router.Use(AuthMi)
	router.Get("/feed", HandleFeedView)

	router.Route("/jobs", func(router chi.Router) {
		router.Get("/", HandleJobsView)
		router.Post("/create", HandleJobsCreate)
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
		router.Post("/{id}/start", HandleStartBuild)
	})

	router.Get("/settings", HandleSettingsGet)
	router.Post("/settings", HandleSettingsPost)

	router.Get("/queue/", HandleQueueGet)

	router.Route("/update", func(router chi.Router) {
		router.Get("/", HandleUpdateGet)
		router.Post("/", HandleUpdatePost)
	})

	router.Route("/keys", func(router chi.Router) {
		router.Get("/", HandleAPIKeysList)
		router.Post("/", HandleAPIKeyCreate)
		router.Delete("/", HandleAPIKeysRevokeBy)
		router.Delete("/{id}", HandleAPIKeyRevoke)
	})
}
