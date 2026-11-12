package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
	"gopkg.in/yaml.v2"
)

// newSettingsFileTestEnv has the admin password, settings with their
// defaults, the group Ops and a fresh limiter
func newSettingsFileTestEnv(t *testing.T) {
	t.Helper()
	newAPIKeyTestEnv(t)
	if err := DB.Update(func(tx *bolt.Tx) error {
		gb := tx.Bucket(GlobalBucket)
		if err := gb.Put([]byte("concurrentBuilds"), IntToByte(2)); err != nil {
			return err
		}
		return gb.Put([]byte("buildHistorySize"), IntToByte(200))
	}); err != nil {
		t.Fatal(err)
	}
	addTestGroup(t, "Ops", Grant{Scopes: []string{ScopeRead, ScopeRun, ScopeWrite}, AllJobs: true})
	oldQueue, oldLimiter := GlobalQueue, apiKeyRequestLimiter
	GlobalQueue = &Queue{}
	apiKeyRequestLimiter = newKeyRequestLimiter(defaultAPIKeyBurst, defaultAPIKeyRate)
	t.Cleanup(func() { GlobalQueue, apiKeyRequestLimiter = oldQueue, oldLimiter })
}

// exported is the settings file without its header, which has the time
func exported(t *testing.T) string {
	t.Helper()
	data, err := exportSettings(time.Now())
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "#") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func importText(t *testing.T, text string) ([]string, error) {
	t.Helper()
	parsed, err := parseSettingsFile([]byte(text))
	if err != nil {
		return nil, err
	}
	return applySettings(parsed, time.Now())
}

// The file has every setting the page changes and none of the secrets, and
// importing it brings them all back
func TestSettingsFileRoundTrip(t *testing.T) {
	newSettingsFileTestEnv(t)
	addTestGroup(t, "Service Team", Grant{Scopes: []string{ScopeRead}, Jobs: []string{"deploy"}, Patterns: []string{"test_*"}})
	createTestKey(t, url.Values{"name": {"ci"}, "scope": {"read"}, "allJobs": {"true"}})
	text, err := exportSettings(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var file SettingsFile
	if err := yaml.UnmarshalStrict(text, &file); err != nil {
		t.Fatalf("the export isn't a settings file: %v\n%s", err, text)
	}
	if *file.ConcurrentBuilds != 2 || *file.BuildHistorySize != 200 || *file.APIKeyRate != 10 || *file.APIKeyBurst != 120 ||
		len(file.SSOGroups) != 2 || file.SSOGroups[0].Name != "Ops" || file.SSOGroups[1].Name != "Service Team" ||
		!slices.Equal(file.SSOGroups[1].Patterns, []string{"test_*"}) || file.SSONoGroup != nil {
		t.Errorf("export = %+v", file)
	}
	if !strings.Contains(string(text), "ssoNoGroup: null") {
		t.Error("the export doesn't say that people in no group can't sign in")
	}
	// Below the header, which says that they aren't there
	for _, secret := range []string{"password", "secretHash", "wk_", "ci"} {
		if strings.Contains(exported(t), secret) {
			t.Errorf("the export has %s", secret)
		}
	}

	before := exported(t)
	settingsRequest(t, HandleSettingsPost, "POST", "", url.Values{"concurrentBuilds": {"5"}, "buildHistorySize": {"50"}, "apiKeyRate": {"3"}, "apiKeyBurst": {"9"}})
	addTestGroup(t, "Strangers", Grant{Scopes: []string{ScopeRead}, AllJobs: true})
	if rec := adminRequest("POST", "/api/nogroup", url.Values{"scope": {"read"}, "allJobs": {"true"}}); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	applied, err := importText(t, string(text))
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if want := []string{"apiKeyBurst", "apiKeyRate", "buildHistorySize", "concurrentBuilds", "ssoGroups", "ssoNoGroup"}; !slices.Equal(applied, want) {
		t.Errorf("applied = %v, want %v", applied, want)
	}
	if after := exported(t); after != before {
		t.Errorf("after the import:\n%s\nwant:\n%s", after, before)
	}
	if burst, rate := apiKeyRequestLimiter.limits(); burst != 120 || rate != 10 || GlobalQueue.concurrentBuilds != 2 {
		t.Errorf("not applied: limiter %d/%d, queue %d", burst, rate, GlobalQueue.concurrentBuilds)
	}
	if keys := listKeysAs(t, adminUser); len(keys.Keys) != 1 {
		t.Error("the import touched the API keys")
	}
}

// A file sets what it has and leaves the rest
func TestSettingsFilePartial(t *testing.T) {
	newSettingsFileTestEnv(t)
	if rec := adminRequest("POST", "/api/nogroup", url.Values{"scope": {"read"}, "allJobs": {"true"}}); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	before := exported(t)
	applied, err := importText(t, "apiKeyRate: 5\n")
	if err != nil || !slices.Equal(applied, []string{"apiKeyRate"}) {
		t.Fatalf("applied %v, %v", applied, err)
	}
	if after := exported(t); after != strings.Replace(before, "apiKeyRate: 10", "apiKeyRate: 5", 1) {
		t.Errorf("more than the rate changed:\n%s", after)
	}

	// Groups are replaced as a whole; null turns the rule off
	if _, err := importText(t, "ssoGroups:\n- name: Dev\n  scopes: [read, run]\n  jobs: [deploy]\nssoNoGroup: null\n"); err != nil {
		t.Fatal(err)
	}
	groups, err := listSSOGroups()
	if err != nil || len(groups) != 1 || groups[0].Name != "Dev" || !slices.Equal(groups[0].Jobs, []string{"deploy"}) {
		t.Errorf("groups = %+v, %v", groups, err)
	}
	if listGroups(t).NoGroup != nil {
		t.Error("ssoNoGroup: null left the rule on")
	}
}

// A file with any mistake changes nothing, and the error names the setting
func TestSettingsFileRefusesMistakes(t *testing.T) {
	newSettingsFileTestEnv(t)
	before := exported(t)
	for _, tt := range []struct{ name, text, want string }{
		{"empty", "", "no settings"},
		{"not YAML", "concurrentBuilds: [", "read the settings file"},
		{"unknown setting", "concurrentbuilds: 3\n", "concurrentbuilds"},
		{"no builds at once", "concurrentBuilds: 0\napiKeyRate: 3\n", "concurrentBuilds"},
		{"a rate over the limit", "apiKeyBurst: 100001\n", "apiKeyBurst"},
		{"a group without scopes", "ssoGroups:\n- name: Dev\n  allJobs: true\n", "ssoGroups Dev: select at least one scope"},
		{"write on some jobs", "ssoGroups:\n- name: Dev\n  scopes: [write]\n  jobs: [deploy]\n", "write"},
		{"a group twice", "ssoGroups:\n- name: Dev\n  scopes: [read]\n  allJobs: true\n- name: DEV\n  scopes: [run]\n  allJobs: true\n", "twice"},
		{"a group without a name", "ssoGroups:\n- scopes: [read]\n  allJobs: true\n", "item 1"},
		{"an unknown scope", "ssoNoGroup:\n  scopes: [admin]\n  allJobs: true\n", "ssoNoGroup"},
		// A list left empty by mistake would lock out every group
		{"groups left empty", "ssoGroups:\n", "ssoGroups: []"},
		{"groups set to null", "ssoGroups: null\n", "ssoGroups: []"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := importText(t, tt.text)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want one with %q", err, tt.want)
			}
			if after := exported(t); after != before {
				t.Errorf("the settings changed:\n%s", after)
			}
		})
	}
}

// Only the admin password exports and imports, and the import is audited
func TestSettingsFileRoutes(t *testing.T) {
	newSettingsFileTestEnv(t)
	member := loginInGroups(t, "sam@example.com", "Ops")
	if rec := apiCall("GET", "/api/settings/export", member, nil, nil); rec.Code != http.StatusForbidden {
		t.Errorf("export by a member with write: %d, want 403", rec.Code)
	}
	rec := adminRequest("GET", "/api/settings/export", nil)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/yaml") ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), `filename="wakeci-settings-`) {
		t.Fatalf("export: %d %v", rec.Code, rec.Header())
	}
	file := rec.Body.String()

	events, _ := captureAudit(t)
	importAs := func(cookie *http.Cookie, admin bool) int {
		req := httptest.NewRequest("POST", "/api/settings/import", strings.NewReader("apiKeyRate: 7\n"+file))
		req.Header.Set("Content-Type", "application/yaml")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if admin {
			req.SetBasicAuth("", "admin")
		}
		rec := httptest.NewRecorder()
		apiRouter().ServeHTTP(rec, req)
		return rec.Code
	}
	if code := importAs(member, false); code != http.StatusForbidden {
		t.Errorf("import by a member with write: %d, want 403", code)
	}
	if code := importAs(nil, true); code != http.StatusBadRequest {
		t.Errorf("import of a file with apiKeyRate twice: %d, want 400", code)
	}
	file = strings.Replace(file, "apiKeyRate: 10", "apiKeyRate: 7", 1)
	req := httptest.NewRequest("POST", "/api/settings/import", strings.NewReader(file))
	req.SetBasicAuth("", "admin")
	rec = httptest.NewRecorder()
	apiRouter().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"apiKeyRate"`) {
		t.Errorf("import: %d %s", rec.Code, rec.Body.String())
	}
	if _, rate := apiKeyRequestLimiter.limits(); rate != 7 {
		t.Errorf("rate after the import = %d, want 7", rate)
	}
	var ok auditEvent
	for _, event := range events() {
		if event["event"] == "settings.import" && event["outcome"] == "ok" {
			ok = event
		}
	}
	if ok == nil || !strings.Contains(ok["applied"], "ssoGroups") {
		t.Errorf("audit = %+v", events())
	}
}

// --import-settings fills the database of a server that never ran, and its
// first start keeps the imported values; a running server's database is
// refused
func TestImportSettingsFile(t *testing.T) {
	newSettingsFileTestEnv(t)
	dir := t.TempDir()
	Config = &WakeConfig{WorkDir: filepath.Join(dir, "new") + string(os.PathSeparator)}
	path := filepath.Join(dir, "settings.yaml")
	if err := os.WriteFile(path, []byte("concurrentBuilds: 4\nssoGroups:\n- name: Dev\n  scopes: [read]\n  allJobs: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	applied, err := importSettingsFile(path)
	if err != nil || !slices.Equal(applied, []string{"concurrentBuilds", "ssoGroups"}) {
		t.Fatalf("import: %v, %v", applied, err)
	}

	db, err := bolt.Open(Config.WorkDir+"wakeci.db", 0644, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	// The first start of the server
	if err := db.Update(bootstrapDB); err != nil {
		t.Fatal(err)
	}
	if err := db.View(func(tx *bolt.Tx) error {
		gb := tx.Bucket(GlobalBucket)
		concurrent, _ := ByteToInt(gb.Get([]byte("concurrentBuilds")))
		history, _ := ByteToInt(gb.Get([]byte("buildHistorySize")))
		if concurrent != 4 || history != 200 || gb.Get([]byte("password")) == nil ||
			tx.Bucket(SSOGroupsBucket).Get(ssoGroupKey("dev")) == nil {
			t.Errorf("after the first start: %d concurrent builds, %d kept, password %t", concurrent, history, gb.Get([]byte("password")) != nil)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// The database is open, as in a running server
	importLockTimeout = 100 * time.Millisecond
	t.Cleanup(func() { importLockTimeout = 2 * time.Second })
	if _, err := importSettingsFile(path); err == nil || !strings.Contains(err.Error(), "stop wakeci first") {
		t.Errorf("import while the database is in use: %v", err)
	}
}

// The settings page shows what Wakefile.yaml set: the secrets file and how
// many secrets it has, never their values; the identity provider's
// certificates with their expiry
func TestServerConfigData(t *testing.T) {
	newSSOTestEnv(t)
	if err := DB.Update(func(tx *bolt.Tx) error {
		gb := tx.Bucket(GlobalBucket)
		if err := gb.Put([]byte("concurrentBuilds"), IntToByte(2)); err != nil {
			return err
		}
		return gb.Put([]byte("buildHistorySize"), IntToByte(200))
	}); err != nil {
		t.Fatal(err)
	}
	Config.SSO = SSOConfig{IDPMetadata: "idp.xml", ACSURL: testSSOACSURL, EntityID: testSSOEntityID}
	Config.path, Config.fileFound = "/etc/wakeci/Wakefile.yaml", true
	Config.SecretsFile, Config.secrets = "/etc/wakeci/secrets.yaml", map[string]string{"TOKEN": "s3cret-value"}
	data := serverConfigData()
	if data.ConfigFile != "/etc/wakeci/Wakefile.yaml" || !data.ConfigFound || data.Secrets != 1 || data.SecretsFile != "/etc/wakeci/secrets.yaml" {
		t.Errorf("server = %+v", data)
	}
	if data.SSO == nil || data.SSO.EntityID != testSSOEntityID || len(data.SSO.Certificates) == 0 || data.SSO.Certificates[0].NotAfter.IsZero() {
		t.Errorf("sso = %+v", data.SSO)
	}
	rec := settingsRequest(t, HandleSettingsGet, "GET", "", nil)
	if strings.Contains(rec.Body.String(), "s3cret-value") || !strings.Contains(rec.Body.String(), `"secrets":1`) {
		t.Errorf("settings = %s", rec.Body.String())
	}
}

// An import error names the setting or the line, never a value: someone may
// import the wrong file, such as the secrets file
func TestSettingsFileErrorsHideValues(t *testing.T) {
	newSettingsFileTestEnv(t)
	for _, text := range []string{
		"apiKeyRate: s3cret-value-1\n",
		"ssoGroups: s3cret-value-1\n",
		"ssoGroups:\n- name: Dev\n  scopes: s3cret-value-1\n",
		"ssoNoGroup: s3cret-value-1\n",
	} {
		_, err := importText(t, text)
		if err == nil || strings.Contains(err.Error(), "s3cret") || !strings.Contains(err.Error(), "line ") {
			t.Errorf("import of %q: %v; want an error naming the line without the value", text, err)
		}
	}
}

// ssoGroups: [] removes every group: the way to say so on purpose
func TestSettingsFileEmptyGroupList(t *testing.T) {
	newSettingsFileTestEnv(t)
	if _, err := importText(t, "ssoGroups: []\n"); err != nil {
		t.Fatal(err)
	}
	if groups, err := listSSOGroups(); err != nil || len(groups) != 0 {
		t.Errorf("groups = %+v, %v; want none", groups, err)
	}
}

// The errors of a group's or the rule's permissions name the group, never
// the value that was wrong
func TestSettingsFileGrantErrorsHideValues(t *testing.T) {
	newSettingsFileTestEnv(t)
	for _, tt := range []struct{ text, want string }{
		{"ssoGroups:\n- name: Dev\n  scopes: [s3cret-value-1]\n  allJobs: true\n", "ssoGroups Dev: unknown scope"},
		{"ssoGroups:\n- name: Dev\n  scopes: [read]\n  jobs: [s3cret/value-1]\n", "ssoGroups Dev: invalid job name or pattern"},
		{"ssoNoGroup:\n  scopes: [read]\n  patterns: ['s3cret\\value-1']\n", "ssoNoGroup: invalid job name or pattern"},
	} {
		_, err := importText(t, tt.text)
		if err == nil || strings.Contains(err.Error(), "s3cret") || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("import of %q: %v; want %q without the value", tt.text, err, tt.want)
		}
	}
}
