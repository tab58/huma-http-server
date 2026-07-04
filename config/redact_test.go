package config

import (
	"strings"
	"testing"
	"time"
)

type redactTestConfig struct {
	Host      string `mapstructure:"HOST"`
	Port      int    `mapstructure:"PORT"`
	JWTSecret string `mapstructure:"JWT_SECRET" sensitive:"true"`
	ShortKey  string `mapstructure:"SHORT_KEY" sensitive:"true"`
	DBPass    int    `mapstructure:"DB_PIN" sensitive:"true"` // non-string secret
}

func TestRedactedForLog(t *testing.T) {
	cfg := redactTestConfig{
		Host:      "example.com",
		Port:      8080,
		JWTSecret: "super-secret-signing-key-ab1de",
		ShortKey:  "tiny",
		DBPass:    123456,
	}

	out, err := redactedForLog(&cfg)
	if err != nil {
		t.Fatalf("redactedForLog: %v", err)
	}

	if strings.Contains(out, "super-secret-signing-key") {
		t.Fatalf("full secret leaked into log output: %s", out)
	}
	if !strings.Contains(out, "*****ab1de") {
		t.Fatalf("expected last-5 suffix '*****ab1de' in output, got: %s", out)
	}
	if strings.Contains(out, "tiny") {
		t.Fatalf("short secret leaked (must be fully masked): %s", out)
	}
	if strings.Contains(out, "123456") {
		t.Fatalf("non-string secret leaked: %s", out)
	}
	if !strings.Contains(out, "example.com") || !strings.Contains(out, "8080") {
		t.Fatalf("non-sensitive fields must print unredacted, got: %s", out)
	}
}

func TestRedactedForLogNestedStructs(t *testing.T) {
	type dbConfig struct {
		Host     string `mapstructure:"DB_HOST"`
		Password string `mapstructure:"DB_PASSWORD" sensitive:"true"`
	}
	type appConfig struct {
		Name      string    `mapstructure:"APP_NAME"`
		DB        dbConfig  `mapstructure:"DB"`
		Replica   *dbConfig `mapstructure:"REPLICA"`
		NoReplica *dbConfig `mapstructure:"NO_REPLICA"`
		StartedAt time.Time `mapstructure:"STARTED_AT"`
	}

	cfg := appConfig{
		Name:      "svc",
		DB:        dbConfig{Host: "db.internal", Password: "nested-secret-value-xy9zq"},
		Replica:   &dbConfig{Host: "replica.internal", Password: "replica-secret-value-qw8er"},
		StartedAt: time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC),
	}

	out, err := redactedForLog(&cfg)
	if err != nil {
		t.Fatalf("redactedForLog: %v", err)
	}

	if strings.Contains(out, "nested-secret-value") || strings.Contains(out, "replica-secret-value") {
		t.Fatalf("nested secret leaked into log output: %s", out)
	}
	if !strings.Contains(out, "*****xy9zq") || !strings.Contains(out, "*****qw8er") {
		t.Fatalf("nested secrets not redacted with last-5 suffix: %s", out)
	}
	if !strings.Contains(out, "db.internal") || !strings.Contains(out, "replica.internal") {
		t.Fatalf("nested non-sensitive fields must print unredacted: %s", out)
	}
	if !strings.Contains(out, `"NO_REPLICA":null`) {
		t.Fatalf("nil nested pointer should render as null: %s", out)
	}
	if !strings.Contains(out, "2026-07-03") {
		t.Fatalf("time.Time should marshal via its own JSON marshaler: %s", out)
	}
}

func TestRedactSecret(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		expected string
	}{
		{"long secret shows last 5", "super-secret-ab1de", "*****ab1de"},
		{"exactly 5 chars fully masked", "ab1de", "*****"},
		{"short fully masked", "abc", "*****"},
		{"empty stays masked", "", "*****"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactSecret(tt.in); got != tt.expected {
				t.Errorf("redactSecret(%q) = %q, want %q", tt.in, got, tt.expected)
			}
		})
	}
}
