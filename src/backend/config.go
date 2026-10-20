package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	yaml "gopkg.in/yaml.v2"
)

// WakeConfig represents global object with configuration
type WakeConfig struct {
	// Port to start the server on
	Port string `yaml:"port"`
	// Hostname for autocert. Active only when port is 443
	Hostname string `yaml:"hostname"`
	// WorkDir contains path to the working directory where db and all
	// build results are stored
	WorkDir string `yaml:"workdir"`
	// Configuration directory - all your job files
	JobDir string `yaml:"jobdir"`
	// Job files extension
	jobsExt string
	// Path to the file with secrets
	SecretsFile string `yaml:"secretsfile"`
	// Map of secrets retrieved from SecretsFile
	secrets map[string]string
	// protectedFiles are the absolute paths of the config and secrets files,
	// which are never jobs even when they sit in JobDir
	protectedFiles []string
	// Timezone for cron jobs (`interval` field in job files)
	Timezone string `yaml:"timezone"`
	// Google enables "Sign in with Google" next to the shared password
	Google GoogleConfig `yaml:"google"`
}

// GoogleConfig configures "Sign in with Google" for Google Workspace
// accounts. Only accounts of a domain in AllowedDomains may sign in; personal
// Google accounts never can.
type GoogleConfig struct {
	// OAuth client ID and secret from the Google Cloud console
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	// RedirectURL overrides the OAuth callback URL, which is otherwise
	// derived from `hostname` or the request: <scheme>://<host>/auth/google/callback
	RedirectURL string `yaml:"redirect_url"`
	// AllowedDomains are Google Workspace domains, matched against the ID
	// token's `hd` claim rather than the email suffix: a personal Google
	// account can carry a verified address on any domain, but never `hd`.
	AllowedDomains []string `yaml:"allowed_domains"`
	// DisablePassword turns the shared password off, so Google is the only
	// way in. A password can't be offboarded: anyone who knows it keeps
	// access after their Google account is gone.
	DisablePassword bool `yaml:"disable_password"`
}

// Enabled reports whether Google sign-in is configured
func (g GoogleConfig) Enabled() bool {
	return g.ClientID != ""
}

// Allows reports whether a Google account may sign in. hostedDomain is the
// ID token's `hd` claim, which Google sets only for Workspace accounts.
func (g GoogleConfig) Allows(hostedDomain string) bool {
	return g.Enabled() && hostedDomain != "" && slices.Contains(g.AllowedDomains, strings.ToLower(hostedDomain))
}

func (g *GoogleConfig) normalize() error {
	if !g.Enabled() {
		return nil
	}
	if g.ClientSecret == "" {
		return errors.New("google.client_secret is required when google.client_id is set")
	}
	for i, domain := range g.AllowedDomains {
		g.AllowedDomains[i] = strings.ToLower(strings.TrimSpace(domain))
	}
	if len(g.AllowedDomains) == 0 {
		return errors.New("google sign-in needs google.allowed_domains: your Google Workspace domains")
	}
	return nil
}

// isProtectedFile reports whether path is the config or the secrets file,
// directly or through a link. Those hold credentials, and with the default
// `jobdir: ./` they sit among the jobs, so they must never be read, written
// or run as one.
func (c *WakeConfig) isProtectedFile(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	info, statErr := os.Stat(abs)
	for _, protected := range c.protectedFiles {
		if abs == protected {
			return true
		}
		if statErr != nil {
			continue
		}
		if protectedInfo, err := os.Stat(protected); err == nil && os.SameFile(info, protectedInfo) {
			return true
		}
	}
	return false
}

// CreateWakeConfig creates new config instance
func CreateWakeConfig(path string) (*WakeConfig, error) {
	config := WakeConfig{}

	// Verify that config file exists
	if _, err := os.Stat(path); err == nil {
		L.Info("using configuration file", "path", path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(data, &config)
		if err != nil {
			return nil, err
		}
	} else if os.IsNotExist(err) {
		L.Info("configuration file not found, using defaults", "path", path)
	} else {
		return nil, err
	}

	// Force defaults
	if config.Port == "" {
		config.Port = "8081"
	}

	if config.WorkDir == "" {
		config.WorkDir = "./wakeci"
	}

	if config.JobDir == "" {
		config.JobDir = "./"
	}

	config.jobsExt = ".yaml"

	err := config.Google.normalize()
	if err != nil {
		return nil, fmt.Errorf("google config: %w", err)
	}

	// Load secrets
	if config.SecretsFile != "" {
		L.Info("loading secrets", "file", config.SecretsFile)
		data, err := os.ReadFile(config.SecretsFile)
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(data, &config.secrets)
		if err != nil {
			return nil, err
		}
	}

	// Clean up the config object
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	// Both dirs are used elsewhere via plain string concatenation
	// (e.g. Config.JobDir+name+Config.jobsExt), so they must always end in a
	// separator - whether or not the configured path was already absolute.
	if !filepath.IsAbs(config.WorkDir) {
		config.WorkDir = filepath.Join(cwd, config.WorkDir)
	}
	if !strings.HasSuffix(config.WorkDir, string(os.PathSeparator)) {
		config.WorkDir += string(os.PathSeparator)
	}
	if !filepath.IsAbs(config.JobDir) {
		config.JobDir = filepath.Join(cwd, config.JobDir)
	}
	if !strings.HasSuffix(config.JobDir, string(os.PathSeparator)) {
		config.JobDir += string(os.PathSeparator)
	}
	for _, file := range []string{path, config.SecretsFile} {
		if file == "" {
			continue
		}
		if !filepath.IsAbs(file) {
			file = filepath.Join(cwd, file)
		}
		config.protectedFiles = append(config.protectedFiles, filepath.Clean(file))
	}
	if cwd+string(os.PathSeparator) == config.JobDir {
		L.Warn("jobdir is the working directory: every YAML file in it with tasks is a job. Consider a dedicated directory", "jobdir", config.JobDir)
	}
	// Log fields individually rather than the whole struct: config.secrets
	// holds decrypted secret values and google.client_secret is a credential;
	// neither must ever be logged.
	L.Debug("current config",
		"port", config.Port,
		"hostname", config.Hostname,
		"workdir", config.WorkDir,
		"jobdir", config.JobDir,
		"secretsfile", config.SecretsFile,
		"timezone", config.Timezone,
		"google", config.Google.Enabled(),
		"googleAllowedDomains", config.Google.AllowedDomains,
	)
	return &config, nil
}
