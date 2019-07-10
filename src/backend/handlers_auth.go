package main

import (
	"net/http"
	"time"

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
	// Create and store session token
	sessionToken, err := uuid.NewV4()
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	expires := time.Now().Add(time.Hour * 24 * 7)
	expiresB, err := expires.GobEncode()
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(SessionBucket))
		return b.Put([]byte(sessionToken.String()), expiresB)
	})
	if err != nil {
		Logger.Println(err)
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
	sessionToken, err := r.Cookie("session")
	if err != nil {
		Logger.Println(err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	err = DB.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(SessionBucket))
		return b.Delete([]byte(sessionToken.Value))
	})
	if err != nil {
		Logger.Println(err)
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
