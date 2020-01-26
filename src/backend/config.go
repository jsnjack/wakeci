package main

import (
	"io/ioutil"
	"os"
	"path/filepath"

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
}

// CreateWakeConfig creates new config instance
func CreateWakeConfig(path string) (*WakeConfig, error) {
	config := WakeConfig{}

	// Verify that config file exists
	if _, err := os.Stat(path); err == nil {
		Logger.Printf("Using configuration file: %s\n", path)
		data, err := ioutil.ReadFile(path)
		if err != nil {
			return nil, err
		}
		err = yaml.Unmarshal(data, &config)
		if err != nil {
			return nil, err
		}
	} else if os.IsNotExist(err) {
		Logger.Printf("Using default configuration: %v\n", err)
	} else {
		return nil, err
	}

	// Clean up the config object
	if config.Port == "" {
		config.Port = "8081"
	}

	if config.WorkDir == "" {
		config.WorkDir = "./wakeci"
	}

	if config.JobDir == "" {
		config.JobDir = "./"
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	if !filepath.IsAbs(config.WorkDir) {
		config.WorkDir = filepath.Join(cwd, config.WorkDir) + "/"
	}
	if !filepath.IsAbs(config.JobDir) {
		config.JobDir = filepath.Join(cwd, config.JobDir) + "/"
	}
	Logger.Printf("Current config: %+v\n", config)
	return &config, nil
}
