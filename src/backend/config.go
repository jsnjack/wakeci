package main

import (
	"fmt"
	"net"
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
	// SSO enables single sign-on through a SAML identity provider such as
	// Google Workspace
	SSO SSOConfig `yaml:"sso"`
}

// SSOConfig sets up single sign-on through a SAML 2.0 identity provider.
// Leaving IDPMetadata empty disables it.
type SSOConfig struct {
	// IDPMetadata is the path to the identity provider's SAML metadata file
	IDPMetadata string `yaml:"idpmetadata"`
	// Groups is a comma-separated list of the groups allowed to log in, as
	// the identity provider names them in the groups attribute
	Groups string `yaml:"groups"`
	// ACSURL is where the identity provider posts the SAML response.
	// Defaults to https://<hostname>/auth/sso/acs
	ACSURL string `yaml:"acsurl"`
	// EntityID identifies wakeci to the identity provider. Defaults to the
	// scheme and host of ACSURL
	EntityID string `yaml:"entityid"`
	// allowedGroups is Groups split into names
	allowedGroups []string
}

// Enabled reports whether single sign-on is configured.
func (s *SSOConfig) Enabled() bool {
	return s.IDPMetadata != ""
}

// resolve checks the SSO settings and fills in the defaults
func (s *SSOConfig) resolve(hostname string) error {
	if !s.Enabled() {
		// Catches a misspelled idpmetadata key, which YAML parsing ignores
		if s.Groups != "" || s.ACSURL != "" || s.EntityID != "" {
			return fmt.Errorf("sso.idpmetadata is required when other sso settings are set")
		}
		return nil
	}
	for _, group := range strings.Split(s.Groups, ",") {
		if group = strings.TrimSpace(group); group != "" {
			s.allowedGroups = append(s.allowedGroups, group)
		}
	}
	if len(s.allowedGroups) == 0 {
		return fmt.Errorf("sso.groups is required when sso.idpmetadata is set")
	}
	if s.ACSURL == "" {
		if hostname == "" {
			return fmt.Errorf("sso.acsurl is required when hostname is not set")
		}
		s.ACSURL = "https://" + hostname + SSOACSPath
	}
	u, err := url.Parse(s.ACSURL)
	if err != nil {
		return fmt.Errorf("parse sso.acsurl: %w", err)
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Path != SSOACSPath ||
		u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("sso.acsurl must look like https://<host>%s, got %q", SSOACSPath, s.ACSURL)
	}
	// The login state cookie is Secure, which browsers only store over https
	// and on localhost
	if u.Scheme == "http" && u.Hostname() != "localhost" && !net.ParseIP(u.Hostname()).IsLoopback() {
		return fmt.Errorf("sso.acsurl must use https unless the host is localhost, got %q", s.ACSURL)
	}
	if s.EntityID == "" {
		s.EntityID = u.Scheme + "://" + u.Host
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

	if err := config.SSO.resolve(config.Hostname); err != nil {
		return nil, fmt.Errorf("sso config: %w", err)
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
	// must never be logged.
	L.Debug("current config",
		"port", config.Port,
		"hostname", config.Hostname,
		"workdir", config.WorkDir,
		"jobdir", config.JobDir,
		"secretsfile", config.SecretsFile,
		"timezone", config.Timezone,
		"sso.idpmetadata", config.SSO.IDPMetadata,
		"sso.groups", config.SSO.allowedGroups,
		"sso.acsurl", config.SSO.ACSURL,
		"sso.entityid", config.SSO.EntityID,
	)
	return &config, nil
}
