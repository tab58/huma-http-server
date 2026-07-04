package config

import (
	"fmt"
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

func bindEnvVars[T any](v *viper.Viper, cfg *T) {
	t := reflect.TypeOf(*cfg)
	for i := range t.NumField() {
		field := t.Field(i)
		if tag := field.Tag.Get("mapstructure"); tag != "" {
			v.BindEnv(tag)
		}
	}
}

// LoadOption configures Load.
type LoadOption func(*loadOptions)

type loadOptions struct {
	configFile string
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
// WithConfigFile is given, a config file. Each call uses a fresh viper
// instance — no state is shared across loads.
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
		fmt.Println("Using config file:", v.ConfigFileUsed())
	} else {
		fmt.Println("no config file specified, using environment variables...")
	}

	// pull in environment variables
	bindEnvVars(v, cfg)
	v.AutomaticEnv()

	// build the config object
	err := v.Unmarshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// print configuration with fields tagged `sensitive:"true"` redacted
	cfgJson, err := redactedForLog(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	fmt.Println("config: ", cfgJson)

	return nil
}
