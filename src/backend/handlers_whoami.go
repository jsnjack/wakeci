package main

import (
	"encoding/binary"
	"net/http"

	bolt "go.etcd.io/bbolt"
)

// WhoAmIData tells a client how it is authenticated, what it may do and
// which server it talks to
type WhoAmIData struct {
	// Auth is how the request was authenticated: apikey, basic (the
	// password as Basic auth), password (a session from the password) or
	// sso
	Auth string `json:"auth"`
	// User is the single sign-on email; for an API key, who created it.
	// Empty for the admin password.
	User string `json:"user"`
	// Admin is true for the admin password, which may do everything
	Admin bool `json:"admin"`
	// Key is the API key the request carries, without its secret
	Key *APIKey `json:"key,omitempty"`
	// Grants are what the request may do, united. A key made by a single
	// sign-on user is also limited by what its creator's groups allow.
	Grants []Grant `json:"grants"`
	// Groups are the single sign-on groups of a session
	Groups []string   `json:"groups,omitempty"`
	Server ServerInfo `json:"server"`
}

// ServerInfo says which wakeci server answered, so a client with several
// can tell them apart
type ServerInfo struct {
	Hostname string `json:"hostname"`
	Version  string `json:"version"`
	// FirstBuild and LastBuild are the IDs of the builds the server holds,
	// 0 if it has none
	FirstBuild int `json:"firstBuild"`
	LastBuild  int `json:"lastBuild"`
}

// HandleWhoAmI tells the client how it is authenticated and what it may do
// @Summary      Tell the client how it is authenticated, what it may do and which server answers
// @Description  Any API key or login; no scope needed. server holds the range of build IDs this server has, to tell servers apart.
// @Tags         auth
// @Produce      json
// @Success      200      {object}   WhoAmIData
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      429      {string}   string
// @Failure      500      {string}   string
// @Router       /api/whoami [get]
func HandleWhoAmI(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	access := accessFromContext(r.Context())
	if access == nil {
		writePlainText(w, logger, http.StatusForbidden, "Forbidden")
		return
	}
	data := WhoAmIData{
		Admin:  access.Full,
		Grants: access.Grants,
		Groups: access.groups,
		Key:    apiKeyFromContext(r.Context()),
		Server: ServerInfo{Hostname: auditServerName(Config.Hostname), Version: Version},
	}
	switch _, _, basic := r.BasicAuth(); {
	case data.Key != nil:
		data.Auth, data.User = "apikey", data.Key.CreatedBy
	case basic:
		data.Auth = "basic"
	default:
		user, err := requestUser(r)
		if err != nil {
			writePlainText(w, logger, http.StatusUnauthorized, "The session ended; log in again")
			return
		}
		data.Auth, data.User = sessionAuth(user), user
	}
	err := DB.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(HistoryBucket).Cursor()
		if first, _ := c.First(); first != nil {
			data.Server.FirstBuild = int(binary.BigEndian.Uint64(first))
		}
		if last, _ := c.Last(); last != nil {
			data.Server.LastBuild = int(binary.BigEndian.Uint64(last))
		}
		return nil
	})
	if err != nil {
		logger.Error("read build range", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, logger, data)
}
