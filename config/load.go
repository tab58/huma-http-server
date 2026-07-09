package config

import (
	"fmt"
	"log/slog"
	"reflect"

	"github.com/spf13/viper"
	"github.com/tab58/huma-http-server/utils"
)

// AppMode is the mode the application is running in.
type AppMode string

const (
	// AppModeDevelopment is the development mode. This is a debug mode.
	AppModeDevelopment AppMode = "development"

	// AppModeProduction is the production mode. This is a production mode.
	AppModeProduction AppMode = "production"
)

// bindEnvVarsAndDefaults binds each `mapstructure`-tagged field to its env
// var and registers its `default` tag value (if any) as the viper default.
// Precedence stays viper-native: env > config file > default.
func bindEnvVarsAndDefaults[T any](v *viper.Viper, cfg *T) {
	t := reflect.TypeOf(*cfg)
	for i := range t.NumField() {
		field := t.Field(i)
		tag := field.Tag.Get("mapstructure")
		if tag == "" {
			continue
		}
		v.BindEnv(tag)
		if def := field.Tag.Get("default"); def != "" {
			v.SetDefault(tag, def)
		}
	}
}

// LoadOption configures Load.
type LoadOption func(*loadOptions)

type loadOptions struct {
	configFile string
	dumpConfig bool
}

// WithConfigDump logs the loaded config via slog at Info level, with fields
// tagged `sensitive:"true"` redacted. Off by default — a library should not
// print, and untagged secrets would leak.
func WithConfigDump() LoadOption {
	return func(o *loadOptions) {
		o.dumpConfig = true
	}
}

// WithConfigFile reads configuration from the given file (any format viper
// supports). Environment variables still take precedence. A missing or
// unreadable file is an error.
func WithConfigFile(path string) LoadOption {
	return func(o *loadOptions) {
		o.configFile = path
	}
}

// Load loads the configuration from environment variables and, when
// WithConfigFile is given, a config file. Fields with a `default:"..."`
// struct tag fall back to that value (precedence: env > file > default).
// Each call uses a fresh viper instance — no state is shared across loads.
func Load[T any](cfg *T, options ...LoadOption) error {
	if !utils.IsStructOrStructPtr(cfg) {
		return fmt.Errorf("config must be a struct or pointer to a struct")
	}

	opts := loadOptions{}
	for _, option := range options {
		option(&opts)
	}

	v := viper.New()
	if opts.configFile != "" {
		v.SetConfigFile(opts.configFile)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read config file %s: %w", opts.configFile, err)
		}
	}

	// pull in environment variables and `default` tag values
	bindEnvVarsAndDefaults(v, cfg)
	v.AutomaticEnv()

	// build the config object
	err := v.Unmarshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// opt-in: log the config with fields tagged `sensitive:"true"` redacted
	if opts.dumpConfig {
		cfgJson, err := redactedForLog(cfg)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
		slog.Info("config loaded", "file", v.ConfigFileUsed(), "config", cfgJson)
	}

	return nil
}
