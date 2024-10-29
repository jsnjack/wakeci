package main

import (
	"encoding/json"
	"log"
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
// @Param        password           formData      string   false  "Set password"
// @Param        concurrentBuilds   formData      string   false  "Set max number of concurrent builds"
// @Param        buildHistorySize   formData      string   false  "Set max number of preserved builds"
// @Success      200      {string}   string
// @Failure      500      {string}   string
// @Router       /settings/ [post]
func HandleSettingsPost(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	// Password
	password := r.FormValue("password")
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
			return nil
		})
		if err != nil {
			logger.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte(err.Error()))
			return
		}
	}

	// Number of concurrent builds
	cb := r.FormValue("concurrentBuilds")
	cbInt, err := strconv.Atoi(cb)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(err.Error()))
		return
	}
	GlobalQueue.SetConcurrency(cbInt)

	// Build history size
	bhs := r.FormValue("buildHistorySize")
	bhsInt, err := strconv.Atoi(bhs)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(err.Error()))
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
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(err.Error()))
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
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
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
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(err.Error()))
		return
	}

	payloadB, err := json.Marshal(settings)
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(payloadB)
}
