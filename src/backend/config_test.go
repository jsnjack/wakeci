package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		wantACSURL   string
		wantEntityID string
	}{
		{name: "not configured", content: "port: \"8081\"\n"},
		{
			name:         "defaults from hostname",
			content:      "hostname: ci.example.com\n" + sso,
			wantEnabled:  true,
			wantACSURL:   "https://ci.example.com/auth/sso/acs",
			wantEntityID: "https://ci.example.com",
		},
		{
			name:         "explicit ACS URL and entity ID",
			content:      sso + "  acsurl: http://localhost:8080/auth/sso/acs\n  entityid: wakeci\n",
			wantEnabled:  true,
			wantACSURL:   "http://localhost:8080/auth/sso/acs",
			wantEntityID: "wakeci",
		},
		{
			name:         "entity ID from ACS URL",
			content:      sso + "  acsurl: http://localhost:8080/auth/sso/acs\n",
			wantEnabled:  true,
			wantACSURL:   "http://localhost:8080/auth/sso/acs",
			wantEntityID: "http://localhost:8080",
		},
		{
			// The groups are managed in the settings. wakeci 2.41 required
			// sso.groups, so a file kept for a rollback still has it
			name:         "groups left by wakeci 2.41",
			content:      "hostname: ci.example.com\n" + sso + "  groups: Developers, Ops\n",
			wantEnabled:  true,
			wantACSURL:   "https://ci.example.com/auth/sso/acs",
			wantEntityID: "https://ci.example.com",
		},
		{
			name:    "no hostname or ACS URL",
			content: sso,
			wantErr: true,
		},
		{
			name:         "plain http on loopback IP",
			content:      sso + "  acsurl: http://127.0.0.1:8080/auth/sso/acs\n",
			wantEnabled:  true,
			wantACSURL:   "http://127.0.0.1:8080/auth/sso/acs",
			wantEntityID: "http://127.0.0.1:8080",
		},
		{
			name:    "plain http on another host",
			content: sso + "  acsurl: http://ci.example.com/auth/sso/acs\n",
			wantErr: true,
		},
		{
			name:    "ACS URL without scheme",
			content: sso + "  acsurl: ci.example.com/auth/sso/acs\n",
			wantErr: true,
		},
		{
			name:    "ACS URL with another scheme",
			content: sso + "  acsurl: ftp://ci.example.com/auth/sso/acs\n",
			wantErr: true,
		},
		{
			name:    "ACS URL without host",
			content: sso + "  acsurl: https:///auth/sso/acs\n",
			wantErr: true,
		},
		{
			name:    "ACS URL with another path",
			content: sso + "  acsurl: https://ci.example.com/acs\n",
			wantErr: true,
		},
		{
			name:    "ACS URL with query",
			content: sso + "  acsurl: https://ci.example.com/auth/sso/acs?x=1\n",
			wantErr: true,
		},
		{
			name:    "misspelled idpmetadata",
			content: "hostname: ci.example.com\nsso:\n  idpmetdata: idp.xml\n  entityid: wakeci\n",
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
			if config.SSO.ACSURL != tt.wantACSURL {
				t.Errorf("ACSURL = %q, want %q", config.SSO.ACSURL, tt.wantACSURL)
			}
			if config.SSO.EntityID != tt.wantEntityID {
				t.Errorf("EntityID = %q, want %q", config.SSO.EntityID, tt.wantEntityID)
			}
		})
	}
}

func TestCreateWakeConfigDrainTimeout(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    time.Duration
		wantErr bool
	}{
		{"default", "", 30 * time.Minute, false},
		{"set", "draintimeout: 90s\n", 90 * time.Second, false},
		{"zero aborts at once", "draintimeout: 0s\n", 0, false},
		{"not a duration", "draintimeout: soon\n", 0, true},
		{"negative", "draintimeout: -1m\n", 0, true},
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
					t.Fatal("want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateWakeConfig: %v", err)
			}
			if config.drainTimeout != tt.want {
				t.Errorf("drainTimeout = %v, want %v", config.drainTimeout, tt.want)
			}
		})
	}
}
