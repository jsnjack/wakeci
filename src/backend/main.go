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

//go:embed all:assets
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
// @description Authenticate with an API key (Authorization: Bearer wk_...), or with the admin password as Basic auth with an empty user name.
// @description A key may only use the routes that name the scope it needs ("API key scope"), and only on its jobs.
// @description Missing or wrong credentials get 401, a request the key or user may not make gets 403, an unknown route or build 404.

// @BasePath /

// @securityDefinitions.basic PasswordAuth
// @description The admin password, with an empty user name

// @securityDefinitions.apikey APIKey
// @in header
// @name Authorization
// @description An API key, sent as "Bearer wk_..."

// @security APIKey
// @security PasswordAuth
func main() {
	cleanup := initApp()
	tasksLeft := run(cleanup)
	// Read once run returned. Only a stop signal cancels it on the way out
	// (Queue.noteStopSignal), not, say, a request that outlasts the HTTP
	// shutdown
	restart, err := exitPlan(GlobalQueue.Status().RestartVersion, tasksLeft)
	if err != nil {
		L.Error("not restarting", "err", err)
		cleanup()
		os.Exit(1)
	}
	if restart {
		// Keeps the PID, so a service manager sees the same process
		L.Warn("restarting", "binary", executablePath)
		err := syscall.Exec(executablePath, os.Args, os.Environ())
		L.Error("restart", "err", err)
		cleanup()
		os.Exit(1)
	}
	cleanup()
}

// exitPlan says whether wakeci re-executes into the update restartVersion
// once run returned. With tasks left that it couldn't stop, it doesn't: they
// would run beside the builds the new process restores. The error asks main
// to exit with status 1 instead, so a service manager ends them and starts
// the new binary
func exitPlan(restartVersion string, tasksLeft bool) (bool, error) {
	if restartVersion == "" {
		return false, nil
	}
	if tasksLeft {
		return false, fmt.Errorf("tasks still run; start wakeci again to run %s", restartVersion)
	}
	return true, nil
}

// run starts the server and returns once it stopped, and whether tasks still
// ran that it couldn't stop. wakeci starts again if an update planned it
// (Queue.BeginStop)
func run(cleanup func()) (tasksLeft bool) {
	fatal := func(msg string, err error) {
		L.Error(msg, "err", err)
		cleanup()
		os.Exit(1)
	}

	var err error
	// Signal handling, set up below, ends last, once the database closed, so
	// a further signal can't cut the steps before short
	var stopSignalHandling func()
	defer func() {
		if stopSignalHandling != nil {
			stopSignalHandling()
		}
	}()
	// interrupt gets the stop signals once the queue noted them
	var interrupt <-chan os.Signal
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

		_, err = tx.CreateBucketIfNotExists(SSOGroupsBucket)
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
	// Last before the database closes
	defer GlobalSessionStorage.saveUnsaved()

	if err := seedSSOGroups(Config.SSO.allowedGroups, time.Now()); err != nil {
		fatal("add the groups of sso.groups", err)
	}
	if Config.SSO.Enabled() {
		GlobalSSOAuth, err = NewSSOAuth(Config.SSO, knownSSOGroup)
		if err != nil {
			fatal("set up sso", err)
		}
		L.Info("sso enabled")
	}

	GlobalQueue, err = CreateQueue()
	if err != nil {
		fatal("create queue", err)
	}

	// Builds broadcast their updates, also the restored ones
	WSHub = newHub()
	go WSHub.run()
	GlobalSessionStorage.OnEnd = WSHub.CloseSessions
	onSSOAccessChange = WSHub.CloseRestricted

	// Runs after the cron jobs ended, so builds they queued are saved with
	// their on_pending tasks done. A further signal ends the wait
	defer func() {
		if !GlobalQueue.waitQueuedHooks(stopPendingTasksTimeout, interrupt) {
			L.Warn("stopping before the on_pending tasks of queued builds ended")
		}
		// Before the database closes, so the builds it ends are saved. After
		// an update, a task left running would run beside the restored builds
		if !GlobalQueue.stopTasks(stopNote, stopTasksTimeout) {
			L.Warn("stopping while tasks still run; they may outlive wakeci")
			tasksLeft = true
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
	router.Route("/storage", storageRoutes(storage))

	router.Route("/docs", docsRoutes)

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
	var signalsNoted <-chan struct{}
	interrupt, signalsNoted = forwardStopSignals(GlobalQueue, signals)
	stopSignalHandling = func() {
		// From here a signal ends the process, which then doesn't restart.
		// One that arrived before is noted before main reads the restart
		signal.Stop(signals)
		close(signals)
		<-signalsNoted
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go drainOnStop(ctx, GlobalQueue, interrupt, Config.drainTimeout, stopHookTimeout, stop)

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
	return tasksLeft
}

// docsRoutes registers the API documentation. It is public, and also
// served to requests with an API key, so agents can read it with their key.
func docsRoutes(router chi.Router) {
	router.Use(StorageSecurityMi)
	router.Get("/api/", HandleAPIDocsView)
	router.Method("GET", "/swagger.json", http.FileServer(http.FS(APIDocs)))
}

// storageRoutes registers the /storage routes, which serve the files of
// builds with storage, for logged-in users and the admin password
// @Summary      Download a file of a build
// @Description  GET /api/build/{id} gives the URL of each task's log (status_update.tasks[].log, absent for a task that wrote none) and of each artifact (status_update.build_artifacts[].url). They are task_<id>.log and artifacts/<filename>, so a job that collects artifacts/** has paths such as artifacts/artifacts/report.json. Supports HEAD and Range, e.g. Range: bytes=-65536 for the end of a log. API key scope: read
// @Tags         build
// @Produce      plain
// @Param        id       path    integer   true   "Build ID"
// @Param        path     path    string    true   "File in the build, may contain /"
// @Param        Range    header  string    false  "Byte range, e.g. bytes=-65536"
// @Success      200      {file}     file
// @Success      206      {file}     file
// @Failure      403      {string}   string
// @Failure      404      {string}   string
// @Router       /storage/build/{id}/{path} [get]
func storageRoutes(storage http.Handler) func(chi.Router) {
	return func(router chi.Router) {
		router.Use(StorageSecurityMi)
		router.Use(AuthMi)
		router.With(requireAccess(ScopeRead, jobFromStorage)).Method("GET", "/build/*", storage)
		router.With(requireAccess(ScopeRead, jobFromStorage)).Method("HEAD", "/build/*", storage)
	}
}

// apiRoutes registers the /api routes for logged-in users and the admin
// password. API keys use newAPIKeyRouter instead. Every route says what it
// needs: a single sign-on user only gets what their groups allow, while the
// admin password passes every check.
func apiRoutes(router chi.Router) {
	// Before AuthMi, so refused changes are audited too
	router.Use(AuditMi)
	router.Use(AuthMi)
	router.With(requireAccess(ScopeRead, nil)).Get("/feed", HandleFeedView)

	router.Route("/jobs", func(router chi.Router) {
		router.With(requireAccess(ScopeRead, nil)).Get("/", HandleJobsView)
		router.With(requireAccess(ScopeWrite, nil)).Post("/create", HandleJobsCreate)
	})

	router.Route("/job", func(router chi.Router) {
		router.With(requireAccess(ScopeRun, jobFromName)).Post("/{name}/run", HandleRunJob)
		router.With(requireAccess(ScopeWrite, jobFromName)).Delete("/{name}", HandleDeleteJob)
		router.With(requireAccess(ScopeWrite, jobFromName)).Post("/{name}", HandleJobPost)
		router.With(requireAccess(ScopeRead, jobFromName)).Get("/{name}", HandleJobGet)
		router.With(requireAccess(ScopeWrite, jobFromName)).Post("/{name}/set_active", HandleJobSetActive)
	})

	router.Route("/build", func(router chi.Router) {
		router.With(requireAccess(ScopeRead, jobFromBuild)).Get("/{id}", HandleGetBuild)
		router.With(requireAccess(ScopeRead, jobFromBuild)).Get("/{id}/summary", HandleBuildSummary)
		router.With(requireAccess(ScopeRead, jobFromBuild)).Get("/{id}/log/{task}", HandleTaskLog)
		router.With(requireAccess(ScopeRun, jobFromBuild)).Post("/{id}/abort", HandleAbortBuild)
		router.With(requireAccess(ScopeRun, jobFromBuild)).Post("/{id}/flush", HandleFlushTaskLogs)
		router.With(requireAccess(ScopeRun, jobFromBuild)).Post("/{id}/start", HandleStartBuild)
	})

	// Write can run any command on the server, so it may change the
	// server's settings too
	router.With(requireAccess(ScopeWrite, nil)).Get("/settings", HandleSettingsGet)
	router.With(requireAccess(ScopeWrite, nil)).Post("/settings", HandleSettingsPost)

	// Every logged-in user: the queue banner shows the server's state
	router.Get("/queue/", HandleQueueGet)

	router.Route("/update", func(router chi.Router) {
		router.With(requireAccess(ScopeWrite, nil)).Get("/", HandleUpdateGet)
		router.With(requireAccess(ScopeWrite, nil)).Post("/", HandleUpdatePost)
	})

	// Every logged-in user manages their own keys; the handlers check the
	// rest
	router.Route("/keys", func(router chi.Router) {
		router.Get("/", HandleAPIKeysList)
		router.Post("/", HandleAPIKeyCreate)
		router.Delete("/", HandleAPIKeysRevokeBy)
		router.Delete("/{id}", HandleAPIKeyRevoke)
	})

	router.Route("/groups", func(router chi.Router) {
		router.With(requireAccess(ScopeWrite, nil)).Get("/", HandleSSOGroupsList)
		router.With(requireAdmin).Post("/", HandleSSOGroupCreate)
		router.With(requireAdmin).Post("/{name}", HandleSSOGroupUpdate)
		router.With(requireAdmin).Delete("/{name}", HandleSSOGroupDelete)
	})
}
