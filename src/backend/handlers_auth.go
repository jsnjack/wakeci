package main

import (
	"log"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"

	bolt "github.com/etcd-io/bbolt"
	"github.com/gofrs/uuid"
	"github.com/julienschmidt/httprouter"
)

// HandleIsLoggedIn returns 200 if user is logged in
func HandleIsLoggedIn(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	// See AuthMi
}

// HandleLogIn verifies password and logs the user in
func HandleLogIn(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	// Create and store session token
	password := r.FormValue("password")

	var hashedPassword []byte

	err := DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(GlobalBucket))
		hashedPassword = b.Get([]byte("password"))
		return nil
	})

	err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
	if err != nil {
		logger.Println(err, password)
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("Incorrect password"))
		return
	}

	sessionToken, err := uuid.NewV4()
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	expires := time.Now().Add(time.Hour * 24 * 7)
	expiresB, err := expires.GobEncode()
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(SessionBucket))
		return b.Put([]byte(sessionToken.String()), expiresB)
	})
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:    "session",
		Value:   sessionToken.String(),
		Expires: expires,
		Path:    "/",
	})
	w.WriteHeader(http.StatusNoContent)
}

// HandleLogOut logs the user out
func HandleLogOut(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	logger, ok := r.Context().Value(HL).(*log.Logger)
	if !ok {
		logger = Logger
	}

	sessionToken, err := r.Cookie("session")
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	err = DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(SessionBucket))
		return b.Delete([]byte(sessionToken.Value))
	})
	if err != nil {
		logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	expires, _ := time.Parse(time.RFC3339, "1970-01-01T00:00:00+00:00")

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:    "session",
		Value:   "delete",
		Expires: expires,
		Path:    "/",
	})
	w.WriteHeader(http.StatusNoContent)
}
