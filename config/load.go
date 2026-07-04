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

func bindEnvVars[T any](cfg *T) {
	t := reflect.TypeOf(*cfg)
	for i := range t.NumField() {
		field := t.Field(i)
		if tag := field.Tag.Get("mapstructure"); tag != "" {
			viper.BindEnv(tag)
		}
	}
}

// Load loads the configuration from the environment variables and the config file.
func Load[T any](cfg *T) error {
	if !utils.IsStructOrStructPtr(cfg) {
		return fmt.Errorf("config must be a struct or pointer to a struct")
	}

	// pull in environment variables
	bindEnvVars(cfg)
	if err := viper.ReadInConfig(); err != nil {
		fmt.Println("config file not found, using environment variables...")
	} else {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
	viper.AutomaticEnv()

	// build the config object
	err := viper.Unmarshal(cfg)
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
