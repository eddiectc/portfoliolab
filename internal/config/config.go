package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	Log       LogConfig       `yaml:"log"`
	Extractors ExtractorConfig `yaml:"extractors"`
}

// ExtractorConfig holds extractor-specific settings.
type ExtractorConfig struct {
	Vanguard VanguardConfig `yaml:"vanguard"`
}

// VanguardConfig holds Vanguard extractor settings.
type VanguardConfig struct {
	// NavHistoryDays is the lookback period for NAV history extraction (default 730 / 2 years).
	NavHistoryDays int `yaml:"nav_history_days"`
}

// ServerConfig holds server-related settings.
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

// DatabaseConfig holds database-related settings.
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level string `yaml:"level"`
}

// Defaults returns a Config with sensible defaults.
func Defaults() Config {
	return Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "0.0.0.0",
		},
		Database: DatabaseConfig{
			Path: "data/portfoliolab.db",
		},
		Log: LogConfig{
			Level: "info",
		},
		Extractors: ExtractorConfig{
			Vanguard: VanguardConfig{
				NavHistoryDays: 730,
			},
		},
	}
}

// Load reads and parses the config file at the given path.
// It returns a Config with defaults merged — only explicitly set
// values in the file override defaults.
func Load(path string) (Config, error) {
	cfg := Defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config file %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config file %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("validate config: %w", err)
	}

	return cfg, nil
}

// Validate checks that the config values are reasonable.
func (c Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port)
	}

	if c.Database.Path == "" {
		return fmt.Errorf("database.path must not be empty")
	}

	// Ensure data directory exists for the database
	dir := c.Database.Path[:lastIndex(c.Database.Path, "/")]
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create database directory %s: %w", dir, err)
	}

	return nil
}

// ServerAddr returns the listen address in "host:port" format.
func (c Config) ServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// LogLevel returns the slog.Level corresponding to the config log level string.
func (c Config) LogLevel() string {
	switch c.Log.Level {
	case "debug", "info", "warn", "error":
		return c.Log.Level
	default:
		return "info"
	}
}

func lastIndex(s, sep string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == sep[0] {
			return i
		}
	}
	return -1
}

// Duration is a helper type alias for convenience.
type Duration = time.Duration
