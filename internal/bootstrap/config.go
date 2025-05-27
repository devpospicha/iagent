package bootstrap

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/devpospicha/iagent/internal/config"
	"github.com/devpospicha/ishared/utils"
)

// LoadAgentConfig loads the agent configuration from a file, environment variables, and command-line flags.
// It applies the overrides in the following order: command-line flags > environment variables > config file.
// The function returns a pointer to the loaded configuration.
func LoadAgentConfig() *config.Config {
	// Flag declarations
	configFlag := flag.String("config", "", "Path to agent config file")
	serverURL := flag.String("server-url", "", "Override server URL")
	interval := flag.Duration("interval", 0, "Override interval (e.g. 5s)")
	host := flag.String("host", "", "Override hostname")

	environment := flag.String("env", "", "Environment (dev, test, prod)")
	logLevel := flag.String("log-level", "", "Log level (debug, info, warn, error)")
	errorLogFile := flag.String("error_log", "", "Path to error log file")
	appLogFile := flag.String("app_log", "", "Path to app log file")
	accessLogFile := flag.String("access_log", "", "Path to access file")
	customTags := flag.String("tags", "", "Comma-separated list of custom tags")

	flag.Parse()

	// Resolve config path
	configPath := resolvePath(*configFlag, "GOSIGHT_AGENT_CONFIG", "application.yaml")
	log.Printf("Loaded config file from: %s", configPath)
	if err := config.EnsureDefaultConfig(configPath); err != nil {
		log.Fatalf("Could not create default config: %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	config.ApplyEnvOverrides(cfg)

	// Apply CLI flag overrides (highest priority)
	if *serverURL != "" {
		cfg.Agent.ServerURL = *serverURL
	}
	if *interval != 0 {
		cfg.Agent.Interval = *interval
	}
	if *host != "" {
		cfg.Agent.HostOverride = *host
	}
	if *environment != "" {
		cfg.Agent.Environment = *environment
	}

	if *logLevel != "" {
		cfg.Logs.LogLevel = *logLevel
	}
	if *appLogFile != "" {
		cfg.Logs.AppLogFile = *appLogFile
	}
	if *errorLogFile != "" {
		cfg.Logs.ErrorLogFile = *errorLogFile
	}
	if *accessLogFile != "" {
		cfg.Logs.AccessLogFile = *errorLogFile
	}
	if *customTags != "" {
		cfg.CustomTags = utils.ParseTagString(*customTags)
	}

	return cfg
}

// resolvePath resolves the path for a given flag value, environment variable, and fallback value.
// It checks if the flag value is set, then checks the environment variable,
// and finally falls back to the provided default value.
func resolvePath(flagVal, envVar, fallback string) string {
	if flagVal != "" {
		return absPath(flagVal)
	}
	if val := os.Getenv(envVar); val != "" {
		utils.Debug("Using %s from environment variable: %s", envVar, val)
		return absPath(val)
	}
	return absPath(fallback)
}

// absPath resolves the absolute path of a given path.
// It uses filepath.Abs to get the absolute path and handles any errors that may occur.
func absPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		log.Fatalf("Failed to resolve path: %v", err)
	}
	return abs
}
