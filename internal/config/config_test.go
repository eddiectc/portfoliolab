package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Database.Path != "data/portfoliolab.db" {
		t.Errorf("expected default db path, got %s", cfg.Database.Path)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected default log level info, got %s", cfg.Log.Level)
	}
}

func TestLoad_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := `
server:
  port: 9090
  host: "127.0.0.1"
database:
  path: "` + tmpDir + `/test.db"
log:
  level: "debug"
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Database.Path != tmpDir+"/test.db" {
		t.Errorf("expected db path %s, got %s", tmpDir+"/test.db", cfg.Database.Path)
	}
	if cfg.Log.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Log.Level)
	}
}

func TestLoad_PartialConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Only override port; rest should use defaults
	content := `
server:
  port: 3000
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Server.Port != 3000 {
		t.Errorf("expected port 3000, got %d", cfg.Server.Port)
	}
	// Defaults should be preserved
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host, got %s", cfg.Server.Host)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected default log level, got %s", cfg.Log.Level)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("expected error for missing config file, got nil")
	}
}

func writeTestConfig(t *testing.T, tmpDir string, content string) string {
	t.Helper()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	return configPath
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	tmpDir := t.TempDir()
	content := `
server:
  port: 9090
  host: "127.0.0.1"
database:
  path: "` + tmpDir + `/test.db"
log:
  level: "debug"
`
	configPath := writeTestConfig(t, tmpDir, content)

	t.Setenv(EnvHost, "10.0.0.5")
	t.Setenv(EnvPort, "7070")
	t.Setenv(EnvDBPath, tmpDir+"/env.db")
	t.Setenv(EnvLogLevel, "warn")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Server.Port != 7070 {
		t.Errorf("expected env port 7070, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "10.0.0.5" {
		t.Errorf("expected env host 10.0.0.5, got %s", cfg.Server.Host)
	}
	if cfg.Database.Path != tmpDir+"/env.db" {
		t.Errorf("expected env db path %s, got %s", tmpDir+"/env.db", cfg.Database.Path)
	}
	if cfg.Log.Level != "warn" {
		t.Errorf("expected env log level warn, got %s", cfg.Log.Level)
	}
}

func TestLoad_EnvFillsUnsetValues(t *testing.T) {
	tmpDir := t.TempDir()
	// File only sets the port; env only sets the host.
	configPath := writeTestConfig(t, tmpDir, "\nserver:\n  port: 3000\n")

	t.Setenv(EnvHost, "10.0.0.9")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Server.Port != 3000 {
		t.Errorf("expected file port 3000, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "10.0.0.9" {
		t.Errorf("expected env host 10.0.0.9, got %s", cfg.Server.Host)
	}
	// Untouched values keep their defaults
	if cfg.Database.Path != "data/portfoliolab.db" {
		t.Errorf("expected default db path, got %s", cfg.Database.Path)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("expected default log level info, got %s", cfg.Log.Level)
	}
}

func TestLoad_EnvInvalidPort(t *testing.T) {
	t.Setenv(EnvPort, "not-a-number")

	_, err := Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err == nil {
		t.Fatal("expected error for invalid PORTFOLIOLAB_PORT, got nil")
	}
	if !strings.Contains(err.Error(), EnvPort) {
		t.Errorf("expected error to mention %s, got %v", EnvPort, err)
	}
}

func TestLoad_MissingFile_EnvOverridesApplied(t *testing.T) {
	t.Setenv(EnvPort, "9090")

	cfg, err := Load("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing config file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("expected env port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected default host, got %s", cfg.Server.Host)
	}
}

func TestLoad_EnvEmptyValueIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := writeTestConfig(t, tmpDir, "\nserver:\n  port: 3000\n")

	t.Setenv(EnvPort, "")

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Server.Port != 3000 {
		t.Errorf("expected file port 3000 (empty env ignored), got %d", cfg.Server.Port)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	content := `
server:
  port: "not-a-number"
  invalid: [
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		wantErr bool
	}{
		{"zero port", 0, true},
		{"negative port", -1, true},
		{"too high port", 65536, true},
		{"valid port", 8080, false},
		{"min valid port", 1, false},
		{"max valid port", 65535, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Server.Port = tt.port
			cfg.Database.Path = t.TempDir() + "/test.db"

			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestValidate_EmptyDBPath(t *testing.T) {
	cfg := Defaults()
	cfg.Database.Path = ""

	err := cfg.Validate()
	if err == nil {
		t.Error("expected error for empty DB path, got nil")
	}
}

func TestServerAddr(t *testing.T) {
	cfg := Defaults()
	addr := cfg.ServerAddr()
	if addr != "0.0.0.0:8080" {
		t.Errorf("expected 0.0.0.0:8080, got %s", addr)
	}
}

func TestLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warn", "warn"},
		{"error", "error"},
		{"invalid", "info"}, // defaults to info
		{"", "info"},        // defaults to info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			cfg := Defaults()
			cfg.Log.Level = tt.input
			if got := cfg.LogLevel(); got != tt.expected {
				t.Errorf("LogLevel(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
