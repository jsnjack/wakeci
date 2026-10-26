package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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
	// Timezone for cron jobs (`interval` field in job files)
	Timezone string `yaml:"timezone"`
	// Google enables "Sign in with Google" for one Google Workspace domain
	Google GoogleConfig `yaml:"google"`
}

// GoogleConfig holds the OAuth client used for Google Workspace sign-in.
// Leaving ClientID empty disables Google sign-in.
type GoogleConfig struct {
	ClientID     string `yaml:"clientid"`
	ClientSecret string `yaml:"clientsecret"`
	// Domain is the Google Workspace domain users must belong to
	Domain string `yaml:"domain"`
	// RedirectURL must match a redirect URI registered for the OAuth client.
	// Defaults to https://<hostname>/auth/google/callback
	RedirectURL string `yaml:"redirecturl"`
}

// Enabled reports whether Google sign-in is configured.
func (g *GoogleConfig) Enabled() bool {
	return g.ClientID != ""
}

// resolve checks the Google settings and fills in the default redirect URL
func (g *GoogleConfig) resolve(hostname string) error {
	if !g.Enabled() {
		// Catches a misspelled clientid key, which YAML parsing ignores
		if g.ClientSecret != "" || g.Domain != "" || g.RedirectURL != "" {
			return fmt.Errorf("google.clientid is required when other google settings are set")
		}
		return nil
	}
	if g.ClientSecret == "" {
		return fmt.Errorf("google.clientsecret is required when google.clientid is set")
	}
	if g.Domain == "" {
		return fmt.Errorf("google.domain is required when google.clientid is set")
	}
	if g.RedirectURL == "" {
		if hostname == "" {
			return fmt.Errorf("google.redirecturl is required when hostname is not set")
		}
		g.RedirectURL = "https://" + hostname + GoogleCallbackPath
	}
	u, err := url.Parse(g.RedirectURL)
	if err != nil {
		return fmt.Errorf("parse google.redirecturl: %w", err)
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Path != GoogleCallbackPath ||
		u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("google.redirecturl must look like https://<host>%s, got %q", GoogleCallbackPath, g.RedirectURL)
	}
	return nil
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

	if err := config.Google.resolve(config.Hostname); err != nil {
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
	// Log fields individually rather than the whole struct: config.secrets
	// and config.Google.ClientSecret must never be logged.
	L.Debug("current config",
		"port", config.Port,
		"hostname", config.Hostname,
		"workdir", config.WorkDir,
		"jobdir", config.JobDir,
		"secretsfile", config.SecretsFile,
		"timezone", config.Timezone,
		"google.clientid", config.Google.ClientID,
		"google.domain", config.Google.Domain,
		"google.redirecturl", config.Google.RedirectURL,
	)
	return &config, nil
}
