package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// HandleIsLoggedIn returns the logged-in Principal; AuthMi answers 403 for
// everyone else
func HandleIsLoggedIn(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	writeJSON(w, logger, principalFrom(r))
}

// AuthMethodsData lists the login methods the login page should offer
type AuthMethodsData struct {
	Password bool `json:"password"`
	Google   bool `json:"google"`
}

// HandleAuthMethods tells the login page which login methods are enabled
func HandleAuthMethods(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	writeJSON(w, logger, AuthMethodsData{Password: passwordLoginEnabled(), Google: Config.Google.Enabled()})
}

// HandleLogIn verifies password and logs the user in
func HandleLogIn(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}
	if !passwordLoginEnabled() {
		writeText(w, logger, http.StatusForbidden, "Password login is disabled, sign in with Google")
		return
	}
	if rejectRateLimited(w, r, logger) {
		return
	}

	// Create and store session token
	password := r.PostFormValue("password")
	if Config.Google.Enabled() && password == "admin" {
		writeText(w, logger, http.StatusForbidden, "Incorrect password")
		return
	}

	hashedPassword, err := readPasswordHash()
	if err != nil {
		logger.Error("read password hash", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	err = bcrypt.CompareHashAndPassword(hashedPassword, []byte(password))
	if err != nil {
		// Never log the submitted password.
		logger.Warn("login failed", "err", err)
		if recordAuthFailure(w, r, logger) {
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusForbidden)
		writeBody(logger, w, []byte("Incorrect password"))
		return
	}

	principal := passwordPrincipal(viaSession)
	principal.passwordFingerprint = passwordFingerprint(hashedPassword)
	c, err := GlobalSessionStorage.New(r, principal)
	if err != nil {
		logger.Error("create session", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	// A password change can finish while bcrypt is checking the old hash.
	currentHash, err := readPasswordHash()
	if err != nil || !bytes.Equal(currentHash, hashedPassword) {
		if deleteErr := GlobalSessionStorage.Delete(c.Value); deleteErr != nil {
			logger.Log(r.Context(), LevelTrace, "discard session after password change", "err", deleteErr)
		}
		if err != nil {
			logger.Error("recheck password hash", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			writeText(w, logger, http.StatusForbidden, "Password changed, please log in again")
		}
		return
	}
	// Set session cookie
	http.SetCookie(w, c)
	w.WriteHeader(http.StatusNoContent)
}

// HandleLogOut logs the user out
func HandleLogOut(w http.ResponseWriter, r *http.Request) {
	logger, ok := r.Context().Value(HL).(*slog.Logger)
	if !ok {
		logger = L
	}

	sessionToken, err := r.Cookie("session")
	if err == nil {
		err = GlobalSessionStorage.Delete(sessionToken.Value)
		if err != nil {
			logger.Error("delete session", "err", err)
		}
		WSHub.Recheck()
	}

	// Set session cookie
	cookie := sessionCookie(r)
	cookie.Value = "delete"
	cookie.Expires = time.Unix(0, 0)
	cookie.MaxAge = -1
	http.SetCookie(w, cookie)
	w.WriteHeader(http.StatusNoContent)
}
