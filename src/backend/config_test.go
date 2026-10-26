package main

import (
	"os"
	"path/filepath"
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

func TestCreateWakeConfigGoogle(t *testing.T) {
	tests := []struct {
		name            string
		content         string
		wantErr         bool
		wantEnabled     bool
		wantRedirectURL string
	}{
		{name: "not configured", content: "port: \"8081\"\n"},
		{
			name:            "redirect URL from hostname",
			content:         "hostname: ci.example.com\ngoogle:\n  clientid: id\n  clientsecret: secret\n  domain: example.com\n",
			wantEnabled:     true,
			wantRedirectURL: "https://ci.example.com/auth/google/callback",
		},
		{
			name:            "explicit redirect URL",
			content:         "google:\n  clientid: id\n  clientsecret: secret\n  domain: example.com\n  redirecturl: http://localhost:8080/auth/google/callback\n",
			wantEnabled:     true,
			wantRedirectURL: "http://localhost:8080/auth/google/callback",
		},
		{
			name:    "missing secret",
			content: "hostname: ci.example.com\ngoogle:\n  clientid: id\n  domain: example.com\n",
			wantErr: true,
		},
		{
			name:    "missing domain",
			content: "hostname: ci.example.com\ngoogle:\n  clientid: id\n  clientsecret: secret\n",
			wantErr: true,
		},
		{
			name:    "no hostname or redirect URL",
			content: "google:\n  clientid: id\n  clientsecret: secret\n  domain: example.com\n",
			wantErr: true,
		},
		{
			name:    "redirect URL without scheme",
			content: "google:\n  clientid: id\n  clientsecret: secret\n  domain: example.com\n  redirecturl: ci.example.com/auth/google/callback\n",
			wantErr: true,
		},
		{
			name:    "redirect URL with another scheme",
			content: "google:\n  clientid: id\n  clientsecret: secret\n  domain: example.com\n  redirecturl: ftp://ci.example.com/auth/google/callback\n",
			wantErr: true,
		},
		{
			name:    "redirect URL without host",
			content: "google:\n  clientid: id\n  clientsecret: secret\n  domain: example.com\n  redirecturl: https:///auth/google/callback\n",
			wantErr: true,
		},
		{
			name:    "redirect URL with another path",
			content: "google:\n  clientid: id\n  clientsecret: secret\n  domain: example.com\n  redirecturl: https://ci.example.com/callback\n",
			wantErr: true,
		},
		{
			name:    "redirect URL with query",
			content: "google:\n  clientid: id\n  clientsecret: secret\n  domain: example.com\n  redirecturl: https://ci.example.com/auth/google/callback?x=1\n",
			wantErr: true,
		},
		{
			name:    "misspelled clientid",
			content: "hostname: ci.example.com\ngoogle:\n  clinetid: id\n  clientsecret: secret\n  domain: example.com\n",
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
			if got := config.Google.Enabled(); got != tt.wantEnabled {
				t.Errorf("Enabled() = %t, want %t", got, tt.wantEnabled)
			}
			if config.Google.RedirectURL != tt.wantRedirectURL {
				t.Errorf("RedirectURL = %q, want %q", config.Google.RedirectURL, tt.wantRedirectURL)
			}
		})
	}
}
