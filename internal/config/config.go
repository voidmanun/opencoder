package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"encoding/json"
	"github.com/joho/godotenv"
)

// Config holds all configuration for the dingtalk-bridge service.
type Config struct {
	// DingTalk App Credentials
	DingTalkClientID     string
	DingTalkClientSecret string

	// OpenCode Server Configuration
	OpenCodeServerURL      string
	OpenCodeServerPassword string
	OpenCodeServerUsername string

	// OpenCode Model Configuration (optional)
	OpenCodeProviderID string
	OpenCodeModelID    string
	OpenCodeAgent      string

	// Bridge Configuration
	BridgeHost    string
	BridgePort    int
	BridgeMode    string // "advanced" or "mvp"
	BridgeWorkDir string

	// Session Configuration
	SessionStorePath string
	SessionTimeout   time.Duration

	// Security Configuration
	DingTalkUserWhitelistPath string
	ToolWhitelistPath         string

	// Logging Configuration
	LogLevel    string
	LogFilePath string
	// Loaded runtime configurations
	UserWhitelist *UserWhitelistConfig
}

// Load reads configuration from environment variables and .env file.
// Priority: env vars > .env file > defaults.
func Load() (*Config, error) {
	// Load .env file if exists (ignore error if not found)
	_ = godotenv.Load()

	cfg := &Config{
		// DingTalk credentials (required)
		DingTalkClientID:     getEnvString("DINGTALK_CLIENT_ID", ""),
		DingTalkClientSecret: getEnvString("DINGTALK_CLIENT_SECRET", ""),

		// OpenCode server (required)
		OpenCodeServerURL:      getEnvString("OPENCODE_SERVER_URL", "http://127.0.0.1:4096"),
		OpenCodeServerPassword: getEnvString("OPENCODE_SERVER_PASSWORD", ""),
		OpenCodeServerUsername: getEnvString("OPENCODE_SERVER_USERNAME", "opencode"),

		// OpenCode model configuration (optional)
		OpenCodeProviderID: getEnvString("OPENCODE_PROVIDER_ID", ""),
		OpenCodeModelID:    getEnvString("OPENCODE_MODEL_ID", ""),
		OpenCodeAgent:      getEnvString("OPENCODE_AGENT", ""),

		// Bridge settings
		BridgeHost:    getEnvString("BRIDGE_HOST", "127.0.0.1"),
		BridgePort:    getEnvInt("BRIDGE_PORT", 8080),
		BridgeMode:    getEnvString("BRIDGE_MODE", "advanced"),
		BridgeWorkDir: getEnvString("BRIDGE_WORKDIR", "."),

		// Session settings
		SessionStorePath: getEnvString("SESSION_STORE_PATH", "~/.dingtalk-bridge/sessions.json"),
		SessionTimeout:   time.Duration(getEnvInt("SESSION_TIMEOUT", 28800)) * time.Second, // 8 hours default

		// Security settings
		DingTalkUserWhitelistPath: getEnvString("DINGTALK_USER_WHITELIST_PATH", "./config/user_whitelist.json"),
		ToolWhitelistPath:         getEnvString("TOOL_WHITELIST_PATH", "./config/tool_whitelist.json"),

		// Logging settings
		LogLevel:    getEnvString("LOG_LEVEL", "info"),
		LogFilePath: getEnvString("LOG_FILE_PATH", "~/.dingtalk-bridge/bridge.log"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate ensures all required configuration is present and valid.
func (c *Config) Validate() error {
	var errs []string

	// Required DingTalk credentials
	if c.DingTalkClientID == "" {
		errs = append(errs, "DINGTALK_CLIENT_ID is required")
	}
	if c.DingTalkClientSecret == "" {
		errs = append(errs, "DINGTALK_CLIENT_SECRET is required")
	}

	// Required OpenCode server password
	if c.OpenCodeServerPassword == "" {
		errs = append(errs, "OPENCODE_SERVER_PASSWORD is required")
	}

	// Validate bridge mode
	if c.BridgeMode != "advanced" && c.BridgeMode != "mvp" {
		errs = append(errs, "BRIDGE_MODE must be 'advanced' or 'mvp'")
	}

	// Validate security: must bind to localhost only
	if c.BridgeHost != "127.0.0.1" && c.BridgeHost != "localhost" {
		errs = append(errs, "BRIDGE_HOST must be '127.0.0.1' or 'localhost' for security")
	}

	// Validate log level
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[strings.ToLower(c.LogLevel)] {
		errs = append(errs, "LOG_LEVEL must be one of: debug, info, warn, error")
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration errors:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

// ExpandPath expands ~ to home directory in file paths.
func (c *Config) ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// GetExpandedSessionStorePath returns the session store path with ~ expanded.
func (c *Config) GetExpandedSessionStorePath() string {
	return c.ExpandPath(c.SessionStorePath)
}

// GetExpandedLogFilePath returns the log file path with ~ expanded.
func (c *Config) GetExpandedLogFilePath() string {
	return c.ExpandPath(c.LogFilePath)
}

// Helper functions for environment variable parsing

func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// ToolWhitelistConfig defines the structure for tool whitelist configuration.
type ToolWhitelistConfig struct {
	DefaultAllowed   []string            `json:"default_allowed"`
	DefaultBlocked   []string            `json:"default_blocked"`
	UserOverrides    map[string]Override `json:"user_overrides"`
	ProjectOverrides map[string]Override `json:"project_overrides"`
}

// Override defines per-user or per-project tool overrides.
type Override struct {
	Allowed []string `json:"allowed"`
	Blocked []string `json:"blocked"`
}

// UserWhitelistConfig defines the structure for DingTalk user whitelist.
type UserWhitelistConfig struct {
	Enabled bool     `json:"enabled"`
	Users   []string `json:"users"` // DingTalk user IDs (senderStaffId)
}

// LoadUserWhitelist reads the user whitelist configuration from a JSON file.
func LoadUserWhitelist(path string) (*UserWhitelistConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read user whitelist file: %w", err)
	}
	var cfg UserWhitelistConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse user whitelist: %w", err)
	}
	return &cfg, nil
}
