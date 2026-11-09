package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

func newSettingsTestEnv(t *testing.T) {
	t.Helper()
	newTestBuildEnv(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if err := DB.Update(func(tx *bolt.Tx) error {
		gb := tx.Bucket(GlobalBucket)
		for key, value := range map[string][]byte{
			"password": hash, "concurrentBuilds": IntToByte(2), "buildHistorySize": IntToByte(200),
		} {
			if err := gb.Put([]byte(key), value); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	oldQueue, oldSessions := GlobalQueue, GlobalSessionStorage
	GlobalQueue = &Queue{}
	GlobalSessionStorage = &SessionStorage{sessions: make(map[string]session)}
	t.Cleanup(func() {
		GlobalQueue = oldQueue
		GlobalSessionStorage = oldSessions
	})
}

// settingsRequest calls handler logged in as user: a single sign-on email,
// or empty for the admin password
func settingsRequest(t *testing.T, handler http.HandlerFunc, method, user string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	cookie, err := GlobalSessionStorage.New(req, time.Hour, user)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func passwordIs(t *testing.T, password string) bool {
	t.Helper()
	var hash []byte
	if err := DB.View(func(tx *bolt.Tx) error {
		hash = append(hash, tx.Bucket(GlobalBucket).Get([]byte("password"))...)
		return nil
	}); err != nil {
		t.Fatalf("read password: %v", err)
	}
	return bcrypt.CompareHashAndPassword(hash, []byte(password)) == nil
}

// Only the admin password may change the password: an SSO user who set it
// could log in with it and become the admin
func TestSettingsPasswordNeedsAdmin(t *testing.T) {
	newSettingsTestEnv(t)
	form := func(password string) url.Values {
		return url.Values{"password": {password}, "concurrentBuilds": {"3"}, "buildHistorySize": {"100"}}
	}

	rec := settingsRequest(t, HandleSettingsPost, "POST", "def@example.com", form("taken"))
	if rec.Code != http.StatusForbidden || !passwordIs(t, "admin") {
		t.Fatalf("SSO user changing the password: status = %d, still admin %t; want 403 and unchanged",
			rec.Code, passwordIs(t, "admin"))
	}
	// The refused request changes nothing else either
	if GlobalQueue.concurrentBuilds != 0 {
		t.Errorf("concurrentBuilds = %d after a refused request", GlobalQueue.concurrentBuilds)
	}

	if rec := settingsRequest(t, HandleSettingsPost, "POST", "def@example.com", form("")); rec.Code != http.StatusOK {
		t.Errorf("SSO user changing other settings: status = %d, want 200", rec.Code)
	}

	if rec := settingsRequest(t, HandleSettingsPost, "POST", "", form("rotated")); rec.Code != http.StatusOK || !passwordIs(t, "rotated") {
		t.Errorf("admin changing the password: status = %d, changed %t; want 200 and changed", rec.Code, passwordIs(t, "rotated"))
	}

	req := httptest.NewRequest("POST", "/api/settings", strings.NewReader(form("basic").Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth("", "rotated")
	rec = httptest.NewRecorder()
	HandleSettingsPost(rec, req)
	if rec.Code != http.StatusOK || !passwordIs(t, "basic") {
		t.Errorf("Basic auth changing the password: status = %d, changed %t; want 200 and changed", rec.Code, passwordIs(t, "basic"))
	}
}

// A request saves all its settings or none, so a password isn't changed by a
// request that fails, and its audit outcome is true
func TestSettingsAllOrNothing(t *testing.T) {
	setting := func(name string) int {
		var value int
		if err := DB.View(func(tx *bolt.Tx) error {
			var err error
			value, err = ByteToInt(tx.Bucket(GlobalBucket).Get([]byte(name)))
			return err
		}); err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return value
	}
	// 0 concurrent builds would start no build, and a history of 0 builds
	// would delete them all
	invalid := map[string]url.Values{}
	for _, field := range []string{"concurrentBuilds", "buildHistorySize"} {
		for _, value := range []string{"x", "", "0", "-1"} {
			form := url.Values{"password": {"changed"}, "concurrentBuilds": {"3"}, "buildHistorySize": {"100"}}
			form.Set(field, value)
			invalid[fmt.Sprintf("%s %q", field, value)] = form
		}
	}
	for name, form := range invalid {
		t.Run(name, func(t *testing.T) {
			newSettingsTestEnv(t)
			rec := settingsRequest(t, HandleSettingsPost, "POST", "", form)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
			if !passwordIs(t, "admin") {
				t.Error("the password changed")
			}
			if got := setting("concurrentBuilds"); got != 2 || GlobalQueue.concurrentBuilds != 0 {
				t.Errorf("concurrentBuilds = %d saved, %d in the queue; want 2 and unchanged", got, GlobalQueue.concurrentBuilds)
			}
			if got := setting("buildHistorySize"); got != 200 {
				t.Errorf("buildHistorySize = %d, want 200", got)
			}
		})
	}

	newSettingsTestEnv(t)
	form := url.Values{"password": {"changed"}, "concurrentBuilds": {"3"}, "buildHistorySize": {"100"}}
	if rec := settingsRequest(t, HandleSettingsPost, "POST", "", form); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !passwordIs(t, "changed") || setting("concurrentBuilds") != 3 || GlobalQueue.concurrentBuilds != 3 || setting("buildHistorySize") != 100 {
		t.Errorf("valid settings weren't all saved")
	}
}

func TestSettingsGetAdmin(t *testing.T) {
	newSettingsTestEnv(t)
	for user, want := range map[string]bool{"": true, "def@example.com": false} {
		rec := settingsRequest(t, HandleSettingsGet, "GET", user, nil)
		var settings SettingsData
		if err := json.Unmarshal(rec.Body.Bytes(), &settings); err != nil {
			t.Fatalf("decode settings %s: %v", rec.Body.String(), err)
		}
		if settings.Admin != want {
			t.Errorf("user %q: admin = %t, want %t", user, settings.Admin, want)
		}
	}
}

// Sessions survive restarts, so a password change ends the other password
// sessions instead
func TestSettingsPasswordChangeEndsOtherSessions(t *testing.T) {
	newSettingsTestEnv(t)
	other := loginAs(t, adminUser)
	alice := loginAs(t, "alice@example.com")
	current := loginAs(t, adminUser)
	form := url.Values{"password": {"new"}, "concurrentBuilds": {"2"}, "buildHistorySize": {"200"}}
	if rec := apiCall("POST", "/api/settings", current, form, nil); rec.Code != http.StatusOK {
		t.Fatalf("change password: %d %q", rec.Code, rec.Body.String())
	}
	tests := []struct {
		name    string
		cookie  *http.Cookie
		wantErr bool
	}{
		{"session that changed the password", current, false},
		{"other password session", other, true},
		{"single sign-on session", alice, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := GlobalSessionStorage.Verify(tt.cookie.Value); (err != nil) != tt.wantErr {
				t.Errorf("Verify() = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}
