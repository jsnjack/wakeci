package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

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
// @Param        apiKeyRate         formData      string   false  "Requests a second each API key may keep sending, 1 to 100000; unchanged if not sent"
// @Param        apiKeyBurst        formData      string   false  "Requests each API key may send at once, 1 to 100000; unchanged if not sent"
// @Success      200      {string}   string
// @Failure      400      {string}   string
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      429      {string}   string
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
	for _, name := range []string{"apiKeyRate", "apiKeyBurst"} {
		if r.FormValue(name) != "" {
			addAuditAttrs(r, name, auditNumber(r.FormValue(name)))
		}
	}

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
	// The limits of API keys keep their values unless the request sets
	// them: clients written before they were settings don't send them.
	// Those not sent are 0 here and read when saving, under settingsMu, so
	// an import meanwhile isn't undone.
	rate, err := parseLimitSetting(r, "apiKeyRate", 0)
	if err != nil {
		logger.Warn("invalid setting", "err", err)
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	burst, err := parseLimitSetting(r, "apiKeyBurst", 0)
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
		savedBurst, savedRate := apiKeyLimits(tx)
		if rate == 0 {
			rate = savedRate
		}
		if burst == 0 {
			burst = savedBurst
		}
		if err := gb.Put([]byte("apiKeyRate"), IntToByte(rate)); err != nil {
			return err
		}
		if err := gb.Put([]byte("apiKeyBurst"), IntToByte(burst)); err != nil {
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
	apiKeyRequestLimiter.setLimits(burst, rate)
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

// parseLimitSetting reads the form value name, a whole number from 1 to
// maxAPIKeyLimit, or returns current if the form doesn't have it. The error
// never contains the value
func parseLimitSetting(r *http.Request, name string, current int) (int, error) {
	if r.FormValue(name) == "" {
		return current, nil
	}
	value, err := strconv.Atoi(r.FormValue(name))
	if err != nil || value < 1 || value > maxAPIKeyLimit {
		return 0, fmt.Errorf("%s must be a whole number from 1 to %d", name, maxAPIKeyLimit)
	}
	return value, nil
}

// ServerConfigData is what Wakefile.yaml set, for the settings page. It
// names the secrets file and counts its secrets, never shows their values
type ServerConfigData struct {
	ConfigFile string `json:"configFile"`
	// ConfigFound is false when the file wasn't there and defaults apply
	ConfigFound bool   `json:"configFound"`
	Version     string `json:"version"`
	Port        string `json:"port"`
	Hostname    string `json:"hostname"`
	WorkDir     string `json:"workDir"`
	JobDir      string `json:"jobDir"`
	SecretsFile string `json:"secretsFile"`
	Secrets     int    `json:"secrets"`
	// Timezone is for interval schedules; empty for the system's
	Timezone     string `json:"timezone"`
	DrainTimeout string `json:"drainTimeout"`
	// SSO is null while single sign-on isn't set up
	SSO *ServerSSOData `json:"sso" extensions:"x-nullable"`
}

// ServerSSOData is the single sign-on part of Wakefile.yaml
type ServerSSOData struct {
	IDPMetadata string `json:"idpMetadata"`
	ACSURL      string `json:"acsUrl"`
	EntityID    string `json:"entityId"`
	// Certificates sign the identity provider's responses, from its
	// metadata; the metadata needs replacing before they expire
	Certificates []IDPCertificate `json:"certificates"`
}

// IDPCertificate is a signing certificate of the identity provider
type IDPCertificate struct {
	Subject  string    `json:"subject"`
	NotAfter time.Time `json:"notAfter"`
}

// serverConfigData describes the configuration the server runs with
func serverConfigData() ServerConfigData {
	data := ServerConfigData{
		ConfigFile:   Config.path,
		ConfigFound:  Config.fileFound,
		Version:      Version,
		Port:         Config.Port,
		Hostname:     Config.Hostname,
		WorkDir:      Config.WorkDir,
		JobDir:       Config.JobDir,
		SecretsFile:  Config.SecretsFile,
		Secrets:      len(Config.secrets),
		Timezone:     Config.Timezone,
		DrainTimeout: Config.drainTimeout.String(),
	}
	if Config.SSO.Enabled() {
		sso := &ServerSSOData{
			IDPMetadata:  Config.SSO.IDPMetadata,
			ACSURL:       Config.SSO.ACSURL,
			EntityID:     Config.SSO.EntityID,
			Certificates: []IDPCertificate{},
		}
		if GlobalSSOAuth != nil {
			for _, cert := range GlobalSSOAuth.certs {
				sso.Certificates = append(sso.Certificates, IDPCertificate{Subject: cert.Subject.String(), NotAfter: cert.NotAfter})
			}
		}
		data.SSO = sso
	}
	return data
}

// HandleSettingsGet retrieves settings
// @Summary      Retrieve application settings
// @Description  Needs the write scope. Not available with an API key
// @Tags         settings
// @Produce      json
// @Success      200      {object}   SettingsData
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      429      {string}   string
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
		settings.APIKeyBurst, settings.APIKeyRate = apiKeyLimits(tx)
		return nil
	})
	settings.Server = serverConfigData()

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
