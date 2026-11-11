package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestWhoAmI(t *testing.T) {
	router := newAPIKeyTestEnv(t)
	oldVersion := Version
	Version = "9.9.9"
	t.Cleanup(func() { Version = oldVersion })
	addTestGroup(t, "Service Team", Grant{Scopes: []string{ScopeRead}, Jobs: []string{"deploy"}})
	token := createTestKey(t, url.Values{"name": {"ci"}, "scope": {"read", "run"}, "job": {"deploy"}})

	decode := func(t *testing.T, body string) WhoAmIData {
		t.Helper()
		var data WhoAmIData
		if err := json.Unmarshal([]byte(body), &data); err != nil {
			t.Fatalf("decode %q: %v", body, err)
		}
		return data
	}
	server := ServerInfo{Hostname: auditServerName(""), Version: "9.9.9", FirstBuild: 1, LastBuild: 2}

	admin := decode(t, apiCall("GET", "/api/whoami", nil, nil, func(r *http.Request) { r.SetBasicAuth("", "admin") }).Body.String())
	if admin.Auth != "basic" || !admin.Admin || admin.User != "" || admin.Server != server {
		t.Errorf("admin = %+v", admin)
	}

	member := decode(t, sessionRequest("GET", "/api/whoami", loginInGroups(t, "sam@example.com", "Service Team"), nil).Body.String())
	if member.Auth != "sso" || member.Admin || member.User != "sam@example.com" || !slices.Equal(member.Groups, []string{"Service Team"}) ||
		len(member.Grants) != 1 || !slices.Equal(member.Grants[0].Jobs, []string{"deploy"}) {
		t.Errorf("member = %+v", member)
	}

	rec := keyRequest(router, "GET", "/api/whoami", token, nil)
	key := decode(t, rec.Body.String())
	if key.Auth != "apikey" || key.Admin || key.Key == nil || key.Key.Name != "ci" ||
		len(key.Grants) != 1 || !slices.Equal(key.Grants[0].Scopes, []string{ScopeRead, ScopeRun}) || key.Server != server {
		t.Errorf("key = %+v", key)
	}
	if secret := token[strings.LastIndex(token, "_")+1:]; strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), "secretHash") {
		t.Error("whoami shows the key's secret")
	}

	if rec := apiCall("GET", "/api/whoami", nil, nil, nil); rec.Code != http.StatusUnauthorized {
		t.Errorf("without credentials: %d, want 401", rec.Code)
	}
}
