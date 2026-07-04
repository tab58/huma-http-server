package config

import (
	"os"
	"path/filepath"
	"testing"
)

type loadTestConfig struct {
	Host string `mapstructure:"LOADTEST_HOST"`
	Name string `mapstructure:"LOADTEST_NAME"`
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("LOADTEST_HOST", "env-host")

	var cfg loadTestConfig
	if err := Load(&cfg); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "env-host" {
		t.Errorf("Host = %q, want %q", cfg.Host, "env-host")
	}
}

func TestLoadFromConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("LOADTEST_HOST: file-host\nLOADTEST_NAME: file-name\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var cfg loadTestConfig
	if err := Load(&cfg, WithConfigFile(path)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Host != "file-host" || cfg.Name != "file-name" {
		t.Errorf("cfg = %+v, want file-host/file-name", cfg)
	}
}

func TestLoadDoesNotLeakStateAcrossCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("LOADTEST_NAME: leaky\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var first loadTestConfig
	if err := Load(&first, WithConfigFile(path)); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	if first.Name != "leaky" {
		t.Fatalf("first load did not read config file: %+v", first)
	}

	// a second load with no config file must not see the first file's values
	var second loadTestConfig
	if err := Load(&second); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if second.Name != "" {
		t.Fatalf("config file state leaked into a later Load: Name = %q", second.Name)
	}
}

func TestLoadMissingExplicitConfigFileFails(t *testing.T) {
	var cfg loadTestConfig
	if err := Load(&cfg, WithConfigFile("/nonexistent/config.yaml")); err == nil {
		t.Fatal("expected error for missing explicit config file, got nil")
	}
}
