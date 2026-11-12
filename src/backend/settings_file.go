package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
	bolterrors "go.etcd.io/bbolt/errors"
	"gopkg.in/yaml.v2"
)

// The settings as a YAML file, for backups, review in git and moving to
// another server. GET /api/settings/export writes it; POST
// /api/settings/import and --import-settings read it. It never holds the
// password, API keys or sessions. Wakefile.yaml keeps what the server needs
// to start; this file holds what the settings page changes.

// SettingsFile is the settings file. Importing it sets what it has and
// leaves what it doesn't: a file with only ssoGroups changes only the
// groups. ssoGroups replaces every group (ssoGroups: [] removes them all;
// an empty ssoGroups: is refused), and ssoNoGroup: null turns the rule for
// people in no group off.
type SettingsFile struct {
	ConcurrentBuilds *int                `yaml:"concurrentBuilds"`
	BuildHistorySize *int                `yaml:"buildHistorySize"`
	APIKeyRate       *int                `yaml:"apiKeyRate"`
	APIKeyBurst      *int                `yaml:"apiKeyBurst"`
	SSOGroups        []SettingsFileGroup `yaml:"ssoGroups"`
	SSONoGroup       *SettingsFileGrant  `yaml:"ssoNoGroup"`
}

// SettingsFileGrant is a grant as the settings file writes it
type SettingsFileGrant struct {
	Scopes   []string `yaml:"scopes"`
	AllJobs  bool     `yaml:"allJobs,omitempty"`
	Jobs     []string `yaml:"jobs,omitempty"`
	Patterns []string `yaml:"patterns,omitempty"`
}

// SettingsFileGroup is a single sign-on group in the settings file
type SettingsFileGroup struct {
	Name              string `yaml:"name"`
	SettingsFileGrant `yaml:",inline"`
}

// SettingsImportData says what an import changed
type SettingsImportData struct {
	// Applied names the settings the file set
	Applied []string `json:"applied"`
}

// maxSettingsFileSize bounds an imported file
const maxSettingsFileSize = 1 << 20

// importLockTimeout is how long --import-settings waits for the database,
// which a running wakeci holds
var importLockTimeout = 2 * time.Second

func fileGrant(grant Grant) SettingsFileGrant {
	return SettingsFileGrant(grant)
}

// grant checks a grant of the file with the rules of the settings page
func (g SettingsFileGrant) grant() (Grant, error) {
	form := url.Values{"scope": g.Scopes, "allJobs": {fmt.Sprint(g.AllJobs)}, "job": g.Jobs, "pattern": g.Patterns}
	return grantFromForm(form)
}

// exportSettings writes the settings file of the database's settings
func exportSettings(now time.Time) ([]byte, error) {
	var file SettingsFile
	err := DB.View(func(tx *bolt.Tx) error {
		gb := tx.Bucket(GlobalBucket)
		for name, field := range map[string]**int{"concurrentBuilds": &file.ConcurrentBuilds, "buildHistorySize": &file.BuildHistorySize} {
			value, err := ByteToInt(gb.Get([]byte(name)))
			if err != nil {
				return fmt.Errorf("read %s: %w", name, err)
			}
			*field = &value
		}
		burst, rate := apiKeyLimits(tx)
		file.APIKeyRate, file.APIKeyBurst = &rate, &burst
		rule, err := ssoNoGroupRule(tx)
		if err != nil {
			return err
		}
		if rule != nil {
			grant := fileGrant(rule.Grant)
			file.SSONoGroup = &grant
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	groups, err := listSSOGroups()
	if err != nil {
		return nil, err
	}
	file.SSOGroups = []SettingsFileGroup{}
	for _, group := range groups {
		file.SSOGroups = append(file.SSOGroups, SettingsFileGroup{Name: group.Name, SettingsFileGrant: fileGrant(group.Grant)})
	}
	data, err := yaml.Marshal(&file)
	if err != nil {
		return nil, fmt.Errorf("encode settings: %w", err)
	}
	header := fmt.Sprintf("# wakeci %s settings of %s, exported %s.\n"+
		"# Import them in Settings or with --import-settings; the password and\n"+
		"# API keys are never in this file. Importing sets what the file has:\n"+
		"# ssoGroups replaces every group, ssoNoGroup: null turns it off.\n",
		Version, serverName(), now.UTC().Format(time.RFC3339))
	return append([]byte(header), data...), nil
}

// serverName names this server in the file's header
func serverName() string {
	if Config != nil && Config.Hostname != "" {
		return Config.Hostname
	}
	if name, err := os.Hostname(); err == nil {
		return name
	}
	return "this server"
}

// yamlValue is a value the YAML library quotes in its errors, such as
// cannot unmarshal !!str `s3cret-...` into int
var yamlValue = regexp.MustCompile("`[^`]*`")

// hideYAMLValues keeps the line and what was wrong of a YAML error, not the
// value: someone may import the wrong file, such as the secrets file, and
// the error is logged
func hideYAMLValues(err error) string {
	return yamlValue.ReplaceAllString(err.Error(), "a value")
}

// quotedValue is a value the checks of the settings page quote with %q,
// such as unknown scope "x"
var quotedValue = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)

// hideQuotedValues keeps what a grant's check found wrong, not the value
func hideQuotedValues(err error) string {
	return quotedValue.ReplaceAllString(err.Error(), "a value")
}

// parsedSettings is a checked settings file, with which parts it has
type parsedSettings struct {
	file      SettingsFile
	groups    []*SSOGroup
	noGroup   *Grant
	hasGroups bool
	// hasNoGroup is true if the file names ssoNoGroup, also as null
	hasNoGroup bool
}

// parseSettingsFile checks a settings file completely, so an import either
// applies all of it or nothing. Errors name the setting, never a value
func parseSettingsFile(data []byte) (*parsedSettings, error) {
	parsed := &parsedSettings{}
	if err := yaml.UnmarshalStrict(data, &parsed.file); err != nil {
		return nil, fmt.Errorf("read the settings file: %s", hideYAMLValues(err))
	}
	// Which settings the file names: ssoNoGroup: null turns the rule off,
	// while a file without it leaves the rule
	var keys map[string]any
	if err := yaml.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("read the settings file: %s", hideYAMLValues(err))
	}
	if len(keys) == 0 {
		return nil, errors.New("the file has no settings")
	}
	groups, hasGroups := keys["ssoGroups"]
	parsed.hasGroups = hasGroups
	_, parsed.hasNoGroup = keys["ssoNoGroup"]
	// "ssoGroups:" with the list forgotten would remove every group and lock
	// out everyone who signs in with single sign-on
	if hasGroups && groups == nil {
		return nil, errors.New("ssoGroups is empty; write ssoGroups: [] to remove every group")
	}

	f := parsed.file
	for name, value := range map[string]*int{"concurrentBuilds": f.ConcurrentBuilds, "buildHistorySize": f.BuildHistorySize} {
		if value != nil && *value < minCountSetting {
			return nil, fmt.Errorf("%s must be a whole number of at least %d", name, minCountSetting)
		}
	}
	for name, value := range map[string]*int{"apiKeyRate": f.APIKeyRate, "apiKeyBurst": f.APIKeyBurst} {
		if value != nil && (*value < 1 || *value > maxAPIKeyLimit) {
			return nil, fmt.Errorf("%s must be a whole number from 1 to %d", name, maxAPIKeyLimit)
		}
	}
	names := []string{}
	for i, g := range f.SSOGroups {
		name, err := ssoGroupName(g.Name)
		if err != nil {
			return nil, fmt.Errorf("ssoGroups item %d: %w", i+1, err)
		}
		if slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, name) }) {
			return nil, fmt.Errorf("ssoGroups: %s is there twice; names are matched ignoring case", name)
		}
		names = append(names, name)
		grant, err := g.grant()
		if err != nil {
			return nil, fmt.Errorf("ssoGroups %s: %s", name, hideQuotedValues(err))
		}
		parsed.groups = append(parsed.groups, &SSOGroup{Name: name, Grant: grant})
	}
	if f.SSONoGroup != nil {
		grant, err := f.SSONoGroup.grant()
		if err != nil {
			return nil, fmt.Errorf("ssoNoGroup: %s", hideQuotedValues(err))
		}
		parsed.noGroup = &grant
	}
	return parsed, nil
}

// applySettings saves a checked settings file in one transaction and
// applies it, and returns what it set
func applySettings(parsed *parsedSettings, now time.Time) ([]string, error) {
	f := parsed.file
	var applied []string
	// No other request saves settings between this one saving and applying
	// them (see HandleSettingsPost)
	settingsMu.Lock()
	defer settingsMu.Unlock()
	err := DB.Update(func(tx *bolt.Tx) error {
		applied = []string{}
		gb := tx.Bucket(GlobalBucket)
		if f.ConcurrentBuilds != nil {
			if err := saveConcurrency(tx, *f.ConcurrentBuilds); err != nil {
				return err
			}
			applied = append(applied, "concurrentBuilds")
		}
		for name, value := range map[string]*int{"buildHistorySize": f.BuildHistorySize, "apiKeyRate": f.APIKeyRate, "apiKeyBurst": f.APIKeyBurst} {
			if value != nil {
				if err := gb.Put([]byte(name), IntToByte(*value)); err != nil {
					return err
				}
				applied = append(applied, name)
			}
		}
		if parsed.hasGroups {
			b := tx.Bucket(SSOGroupsBucket)
			var old [][]byte
			if err := b.ForEach(func(k, _ []byte) error {
				old = append(old, slices.Clone(k))
				return nil
			}); err != nil {
				return err
			}
			for _, k := range old {
				if err := b.Delete(k); err != nil {
					return err
				}
			}
			for _, group := range parsed.groups {
				group.UpdatedAt = now
				if err := putSSOGroup(b, group); err != nil {
					return err
				}
			}
			// sso.groups mustn't add its groups to the imported ones later
			if err := gb.Put(ssoGroupsSeededKey, []byte("true")); err != nil {
				return err
			}
			applied = append(applied, "ssoGroups")
		}
		if parsed.hasNoGroup {
			if parsed.noGroup == nil {
				if err := gb.Delete(ssoNoGroupKey); err != nil {
					return err
				}
			} else {
				data, err := json.Marshal(&SSOGroup{Grant: *parsed.noGroup, UpdatedAt: now})
				if err != nil {
					return err
				}
				if err := gb.Put(ssoNoGroupKey, data); err != nil {
					return err
				}
			}
			applied = append(applied, "ssoNoGroup")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(applied)
	// The server isn't running when --import-settings imports
	if f.ConcurrentBuilds != nil && GlobalQueue != nil {
		GlobalQueue.applyConcurrency(*f.ConcurrentBuilds)
	}
	if f.APIKeyRate != nil || f.APIKeyBurst != nil {
		if err := DB.View(func(tx *bolt.Tx) error {
			apiKeyRequestLimiter.setLimits(apiKeyLimits(tx))
			return nil
		}); err != nil {
			return nil, err
		}
	}
	if parsed.hasGroups || parsed.hasNoGroup {
		ssoAccessChanged()
	}
	return applied, nil
}

// importSettingsFile imports a settings file into the database of the
// stopped server, for --import-settings, also into a database that doesn't
// exist yet
func importSettingsFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parsed, err := parseSettingsFile(data)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(Config.WorkDir, os.ModePerm); err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	// A running wakeci holds the database's lock
	db, err := bolt.Open(Config.WorkDir+"wakeci.db", 0644, &bolt.Options{Timeout: importLockTimeout})
	if errors.Is(err, bolterrors.ErrTimeout) {
		return nil, errors.New("the database is in use: stop wakeci first")
	}
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	old := DB
	DB = db
	defer func() {
		DB = old
		if closeErr := db.Close(); closeErr != nil {
			L.Error("close db", "err", closeErr)
		}
	}()
	if err := DB.Update(bootstrapDB); err != nil {
		return nil, fmt.Errorf("bootstrap db: %w", err)
	}
	return applySettings(parsed, time.Now())
}

// HandleSettingsExport downloads the settings as YAML
// @Summary      Download the settings as a YAML file
// @Description  Only with the admin password. Concurrency, history size, API key limits, the single sign-on groups and the rule for people in no group; never the password or API keys. POST /api/settings/import and --import-settings read it. Not available with an API key
// @Tags         settings
// @Produce      application/yaml
// @Success      200      {string}   string
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      429      {string}   string
// @Failure      500      {string}   string
// @Router       /api/settings/export [get]
func HandleSettingsExport(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	now := time.Now()
	data, err := exportSettings(now)
	if err != nil {
		logger.Error("export settings", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	name := strings.Map(func(c rune) rune {
		if c == '.' || c == '-' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return c
		}
		return '_'
	}, serverName())
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="wakeci-settings-%s-%s.yaml"`, name, now.Format(time.DateOnly)))
	writeBody(logger, w, data)
}

// HandleSettingsImport imports a settings file
// @Summary      Import a YAML settings file
// @Description  Only with the admin password. Sets what the file has and leaves the rest: ssoGroups replaces every group, ssoNoGroup: null turns the rule for people in no group off. The whole file is checked first, so a file with a mistake changes nothing. Send the file as the body. Not available with an API key
// @Tags         settings
// @Accept       application/yaml
// @Produce      json
// @Param        file     body       string   true   "The settings file, as GET /api/settings/export writes it"
// @Success      200      {object}   SettingsImportData
// @Failure      400      {string}   string
// @Failure      401      {string}   string
// @Failure      403      {string}   string
// @Failure      429      {string}   string
// @Failure      500      {string}   string
// @Router       /api/settings/import [post]
func HandleSettingsImport(w http.ResponseWriter, r *http.Request) {
	logger := requestLogger(r)
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxSettingsFileSize))
	if err != nil {
		writePlainText(w, logger, http.StatusBadRequest, fmt.Sprintf("The file must be at most %d bytes", maxSettingsFileSize))
		return
	}
	parsed, err := parseSettingsFile(bytes.TrimSpace(data))
	if err != nil {
		logger.Warn("invalid settings file", "err", err)
		writePlainText(w, logger, http.StatusBadRequest, err.Error())
		return
	}
	applied, err := applySettings(parsed, time.Now())
	if err != nil {
		logger.Error("import settings", "err", err)
		writePlainText(w, logger, http.StatusInternalServerError, err.Error())
		return
	}
	logger.Info("settings imported", "applied", applied)
	addAuditAttrs(r, auditListAttrs("applied", applied)...)
	writeJSON(w, logger, SettingsImportData{Applied: applied})
}
