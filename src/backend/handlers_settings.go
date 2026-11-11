package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

// HandleSettingsPost saves settings
// @Summary      Update application settings
// @Description  Needs the write scope; only the admin password may change the password. Not available with an API key. All parameters are available as query parameters and as formData
// @Tags         settings
// @Produce      plain
// @Param        password           formData      string   false  "Set password; only with the admin password"
// @Param        concurrentBuilds   formData      string   false  "Set max number of concurrent builds"
// @Param        buildHistorySize   formData      string   false  "Set max number of preserved builds"
// @Success      200      {string}   string
// @Failure      400      {string}   string
// @Failure      403      {string}   string
// @Failure      500      {string}   string
// @Router       /api/settings [post]
func HandleSettingsPost(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	// The audit log says whether the request changes the password (the
	// outcome says if it did), never what to
	addAuditAttrs(r, "passwordChange", r.FormValue("password") != "",
		"concurrentBuilds", auditNumber(r.FormValue("concurrentBuilds")),
		"buildHistorySize", auditNumber(r.FormValue("buildHistorySize")))

	// Everything is checked before anything changes, so a request either
	// saves all its settings or none, and its audit outcome is true
	password := r.FormValue("password")
	if password != "" {
		// Only the admin password may change itself: an SSO user who set it
		// would become the admin
		if user, err := requestUser(r); err != nil || user != "" {
			logger.Warn("password change without the admin password", "user", user, "err", err)
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusForbidden)
			writeBody(logger, w, []byte("Only the admin password can change the password"))
			return
		}
	}

	cbInt, err := parseCountSetting(r, "concurrentBuilds")
	if err != nil {
		// Not the value: it could be a secret typed into the wrong field
		logger.Warn("invalid setting", "err", err)
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	bhsInt, err := parseCountSetting(r, "buildHistorySize")
	if err != nil {
		logger.Warn("invalid setting", "err", err)
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}

	var passwordH []byte
	if password != "" {
		passwordH, err = bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			logger.Error("hash password", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain")
			writeBody(logger, w, []byte(err.Error()))
			return
		}
	}

	// The queue must run with the concurrency the database has, so no other
	// request saves settings between this one saving and applying them
	settingsMu.Lock()
	defer settingsMu.Unlock()
	if passwordH != nil {
		// Logins that checked the old password create their sessions before
		// this, so DeletePasswordSessions ends them
		passwordMu.Lock()
		defer passwordMu.Unlock()
	}
	// One transaction, so either every setting is saved or none
	err = DB.Update(func(tx *bolt.Tx) error {
		gb := tx.Bucket(GlobalBucket)
		if passwordH != nil {
			if err := gb.Put([]byte("password"), passwordH); err != nil {
				return err
			}
		}
		if err := saveConcurrency(tx, cbInt); err != nil {
			return err
		}
		return gb.Put([]byte("buildHistorySize"), IntToByte(bhsInt))
	})
	if err != nil {
		logger.Error("save settings", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	GlobalQueue.applyConcurrency(cbInt)
	if passwordH != nil {
		// Sessions survive restarts, so whoever knew the old password would
		// stay logged in
		ended := GlobalSessionStorage.DeletePasswordSessions(requestSession(r))
		logger.Warn("password changed", "endedSessions", ended)
		addAuditAttrs(r, "endedSessions", ended)
	}
}

// settingsMu orders saving settings with applying them (see
// HandleSettingsPost and Queue.SetConcurrency)
var settingsMu sync.Mutex

// minCountSetting is the smallest number of concurrent builds and of builds
// kept: 0 concurrent builds starts no build, and a history of 0 deletes them
// all
const minCountSetting = 1

// parseCountSetting reads the form value name, which must be a whole number
// of at least minCountSetting. The error never contains the value
func parseCountSetting(r *http.Request, name string) (int, error) {
	value, err := strconv.Atoi(r.FormValue(name))
	if err != nil || value < minCountSetting {
		return 0, fmt.Errorf("%s must be a whole number of at least %d", name, minCountSetting)
	}
	return value, nil
}

// HandleSettingsGet retrieves settings
// @Summary      Retrieve application settings
// @Description  Needs the write scope. Not available with an API key
// @Tags         settings
// @Produce      json
// @Success      200      {object}   SettingsData
// @Failure      500      {string}   string
// @Router       /api/settings [get]
func HandleSettingsGet(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	var settings SettingsData
	// The settings page only offers the password to the admin password
	user, err := requestUser(r)
	settings.Admin = err == nil && user == ""

	err = DB.View(func(tx *bolt.Tx) error {
		gb := tx.Bucket(GlobalBucket)
		cb, err := ByteToInt(gb.Get([]byte("concurrentBuilds")))
		if err != nil {
			return err
		}
		settings.ConcurrentBuilds = cb

		bhs, err := ByteToInt(gb.Get([]byte("buildHistorySize")))
		if err != nil {
			return err
		}
		settings.BuildHistorySize = bhs
		return nil
	})

	if err != nil {
		logger.Error("get settings", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}

	payloadB, err := json.Marshal(settings)
	if err != nil {
		logger.Error("marshal settings", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	writeBody(logger, w, payloadB)
}
