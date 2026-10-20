package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

// HandleSettingsPost saves settings
// @Summary      Update application settings
// @Description  The password is accepted only in the form body. Other settings also accept query parameters.
// @Tags         settings
// @Produce      plain
// @Param        password           formData      string   false  "Set password"
// @Param        concurrentBuilds   formData      string   false  "Set max number of concurrent builds"
// @Param        buildHistorySize   formData      string   false  "Set max number of preserved builds"
// @Success      200      {string}   string
// @Failure      500      {string}   string
// @Router       /settings/ [post]
func HandleSettingsPost(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	// Password
	password := r.PostFormValue("password")
	if password != "" && !passwordLoginEnabled() {
		writeText(w, logger, http.StatusBadRequest, "The shared password is disabled by google.disable_password")
		return
	}
	if password != "" {
		err := DB.Update(func(tx *bolt.Tx) error {
			passwordH, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if err != nil {
				return err
			}

			gb := tx.Bucket(GlobalBucket)
			err = gb.Put([]byte("password"), passwordH)
			if err != nil {
				return err
			}
			// Changing the password is how someone who knew it is cut off, so
			// the keys made with it and its sessions go too, in the same
			// transaction a new key would be stored in (CreateAPIKey)
			owner := ProviderPassword + ":" + passwordUser
			revoked, err := deleteOwnerKeys(tx, owner)
			if err != nil {
				return err
			}
			sessions := GlobalSessionStorage.DeletePrincipal(owner, time.Now())
			logger.Warn("shared password changed", "keysRevoked", revoked, "sessionsEnded", sessions)
			return nil
		})
		if err != nil {
			logger.Error("save password", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain")
			writeBody(logger, w, []byte(err.Error()))
			return
		}
		GlobalSessionStorage.DeletePrincipal(ProviderPassword+":"+passwordUser, time.Now())
		WSHub.Recheck()
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
	GlobalQueue.SetConcurrency(cbInt)

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
	err = DB.Update(func(tx *bolt.Tx) error {
		gb := tx.Bucket(GlobalBucket)
		err = gb.Put([]byte("buildHistorySize"), IntToByte(bhsInt))
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logger.Error("save buildHistorySize", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		writeBody(logger, w, []byte(err.Error()))
		return
	}
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

	err := DB.View(func(tx *bolt.Tx) error {
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
