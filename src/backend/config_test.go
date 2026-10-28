package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCreateWakeConfig(t *testing.T) {
	t.Run("Defaults", func(t *testing.T) {
		config, err := CreateWakeConfig(filepath.Join(t.TempDir(), "missing.yaml"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config.Port != "8081" {
			t.Errorf("expected default port 8081, got %q", config.Port)
		}
		if !filepath.IsAbs(config.WorkDir) {
			t.Errorf("expected WorkDir to be made absolute, got %q", config.WorkDir)
		}
		if !filepath.IsAbs(config.JobDir) {
			t.Errorf("expected JobDir to be made absolute, got %q", config.JobDir)
		}
	})

	t.Run("FromFile", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "Wakefile.yaml")
		content := "port: \"9090\"\nhostname: example.com\ntimezone: UTC\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}

		config, err := CreateWakeConfig(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config.Port != "9090" {
			t.Errorf("expected port 9090, got %q", config.Port)
		}
		if config.Hostname != "example.com" {
			t.Errorf("expected hostname example.com, got %q", config.Hostname)
		}
		if config.Timezone != "UTC" {
			t.Errorf("expected timezone UTC, got %q", config.Timezone)
		}
	})

	t.Run("InvalidYaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "Wakefile.yaml")
		if err := os.WriteFile(path, []byte("port: [this is not valid"), 0644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}

		if _, err := CreateWakeConfig(path); err == nil {
			t.Error("expected an error, got nil")
		}
	})

	t.Run("AbsoluteDirsWithoutTrailingSlash", func(t *testing.T) {
		// WorkDir/JobDir are joined with filenames via plain string
		// concatenation elsewhere (e.g. Config.JobDir+name+Config.jobsExt),
		// so an absolute path configured without a trailing separator must
		// still end up with one - regression test for a bug where only
		// relative paths got normalized this way.
		dir := t.TempDir()
		workDir := filepath.Join(dir, "workdir")
		jobDir := filepath.Join(dir, "jobdir")
		path := filepath.Join(dir, "Wakefile.yaml")
		content := "workdir: " + workDir + "\njobdir: " + jobDir + "\n"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}

		config, err := CreateWakeConfig(path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasSuffix(config.WorkDir, string(os.PathSeparator)) {
			t.Errorf("expected WorkDir to end with a separator, got %q", config.WorkDir)
		}
		if !strings.HasSuffix(config.JobDir, string(os.PathSeparator)) {
			t.Errorf("expected JobDir to end with a separator, got %q", config.JobDir)
		}
	})
}

func TestCreateWakeConfigSSO(t *testing.T) {
	const sso = "sso:\n  idpmetadata: idp.xml\n"
	tests := []struct {
		name         string
		content      string
		wantErr      bool
		wantEnabled  bool
		wantGroups   []string
		wantACSURL   string
		wantEntityID string
	}{
		{name: "not configured", content: "port: \"8081\"\n"},
		{
			name:         "defaults from hostname",
			content:      "hostname: ci.example.com\n" + sso + "  groups: Developers\n",
			wantEnabled:  true,
			wantGroups:   []string{"Developers"},
			wantACSURL:   "https://ci.example.com/auth/sso/acs",
			wantEntityID: "https://ci.example.com",
		},
		{
			name:         "explicit ACS URL and entity ID",
			content:      sso + "  groups: Developers\n  acsurl: http://localhost:8080/auth/sso/acs\n  entityid: wakeci\n",
			wantEnabled:  true,
			wantGroups:   []string{"Developers"},
			wantACSURL:   "http://localhost:8080/auth/sso/acs",
			wantEntityID: "wakeci",
		},
		{
			name:         "entity ID from ACS URL",
			content:      sso + "  groups: Developers\n  acsurl: http://localhost:8080/auth/sso/acs\n",
			wantEnabled:  true,
			wantGroups:   []string{"Developers"},
			wantACSURL:   "http://localhost:8080/auth/sso/acs",
			wantEntityID: "http://localhost:8080",
		},
		{
			name:         "several groups",
			content:      "hostname: ci.example.com\n" + sso + "  groups: Developers, Ops ,,QA,\n",
			wantEnabled:  true,
			wantGroups:   []string{"Developers", "Ops", "QA"},
			wantACSURL:   "https://ci.example.com/auth/sso/acs",
			wantEntityID: "https://ci.example.com",
		},
		{
			name:    "missing groups",
			content: "hostname: ci.example.com\n" + sso,
			wantErr: true,
		},
		{
			name:    "groups without names",
			content: "hostname: ci.example.com\n" + sso + "  groups: \" , \"\n",
			wantErr: true,
		},
		{
			name:    "no hostname or ACS URL",
			content: sso + "  groups: Developers\n",
			wantErr: true,
		},
		{
			name:         "plain http on loopback IP",
			content:      sso + "  groups: Developers\n  acsurl: http://127.0.0.1:8080/auth/sso/acs\n",
			wantEnabled:  true,
			wantGroups:   []string{"Developers"},
			wantACSURL:   "http://127.0.0.1:8080/auth/sso/acs",
			wantEntityID: "http://127.0.0.1:8080",
		},
		{
			name:    "plain http on another host",
			content: sso + "  groups: Developers\n  acsurl: http://ci.example.com/auth/sso/acs\n",
			wantErr: true,
		},
		{
			name:    "ACS URL without scheme",
			content: sso + "  groups: Developers\n  acsurl: ci.example.com/auth/sso/acs\n",
			wantErr: true,
		},
		{
			name:    "ACS URL with another scheme",
			content: sso + "  groups: Developers\n  acsurl: ftp://ci.example.com/auth/sso/acs\n",
			wantErr: true,
		},
		{
			name:    "ACS URL without host",
			content: sso + "  groups: Developers\n  acsurl: https:///auth/sso/acs\n",
			wantErr: true,
		},
		{
			name:    "ACS URL with another path",
			content: sso + "  groups: Developers\n  acsurl: https://ci.example.com/acs\n",
			wantErr: true,
		},
		{
			name:    "ACS URL with query",
			content: sso + "  groups: Developers\n  acsurl: https://ci.example.com/auth/sso/acs?x=1\n",
			wantErr: true,
		},
		{
			name:    "misspelled idpmetadata",
			content: "hostname: ci.example.com\nsso:\n  idpmetdata: idp.xml\n  groups: Developers\n",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Wakefile.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			config, err := CreateWakeConfig(path)
			if tt.wantErr {
				if err == nil {
					t.Error("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := config.SSO.Enabled(); got != tt.wantEnabled {
				t.Errorf("Enabled() = %t, want %t", got, tt.wantEnabled)
			}
			if !slices.Equal(config.SSO.allowedGroups, tt.wantGroups) {
				t.Errorf("allowedGroups = %q, want %q", config.SSO.allowedGroups, tt.wantGroups)
			}
			if config.SSO.ACSURL != tt.wantACSURL {
				t.Errorf("ACSURL = %q, want %q", config.SSO.ACSURL, tt.wantACSURL)
			}
			if config.SSO.EntityID != tt.wantEntityID {
				t.Errorf("EntityID = %q, want %q", config.SSO.EntityID, tt.wantEntityID)
			}
		})
	}
}
