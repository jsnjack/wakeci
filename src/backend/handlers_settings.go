package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

// HandleSettingsPost saves settings
// @Summary      Update application settings
// @Description  All parameters are available as query parameters and as formData
// @Tags         settings
// @Produce      plain
// @Param        password           formData      string   false  "Set password; only with the admin password"
// @Param        concurrentBuilds   formData      string   false  "Set max number of concurrent builds"
// @Param        buildHistorySize   formData      string   false  "Set max number of preserved builds"
// @Success      200      {string}   string
// @Failure      403      {string}   string
// @Failure      500      {string}   string
// @Router       /settings/ [post]
func HandleSettingsPost(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	// The audit log says whether the request changes the password (the
	// outcome says if it did), never what to
	addAuditAttrs(r, "passwordChange", r.FormValue("password") != "",
		"concurrentBuilds", r.FormValue("concurrentBuilds"), "buildHistorySize", r.FormValue("buildHistorySize"))

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

	// Number of concurrent builds
	cb := r.FormValue("concurrentBuilds")
	cbInt, err := strconv.Atoi(cb)
	if err != nil {
		logger.Warn("invalid concurrentBuilds value", "value", cb, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}

	// Build history size
	bhs := r.FormValue("buildHistorySize")
	bhsInt, err := strconv.Atoi(bhs)
	if err != nil {
		logger.Warn("invalid buildHistorySize value", "value", bhs, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
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
}

// HandleSettingsGet retrieves settings
// @Summary      Retrieve application settings
// @Tags         settings
// @Produce      json
// @Success      200      {object}   SettingsData
// @Failure      500      {string}   string
// @Router       /settings/ [get]
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
