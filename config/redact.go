package config

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// redactSecret masks a secret for logging, keeping the last 5 characters so
// operators can confirm the right value loaded. Secrets of 5 characters or
// fewer are fully masked — a suffix would leak most of the value.
func redactSecret(s string) string {
	const visible = 5
	if len(s) <= visible {
		return "*****"
	}
	return "*****" + s[len(s)-visible:]
}

// redactedForLog renders a config struct as JSON with every field tagged
// `sensitive:"true"` redacted, recursing into nested structs. Non-string
// sensitive fields are fully masked.
func redactedForLog(cfg any) (string, error) {
	out, err := redactedFields(reflect.ValueOf(cfg))
	if err != nil {
		return "", err
	}
	rendered, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config for logging: %w", err)
	}
	return string(rendered), nil
}

func redactedFields(v reflect.Value) (map[string]any, error) {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("config must be a struct or pointer to a struct")
	}

	t := v.Type()
	out := make(map[string]any, t.NumField())
	for i := range t.NumField() {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		name := field.Tag.Get("mapstructure")
		if name == "" {
			name = field.Name
		}

		if field.Tag.Get("sensitive") == "true" {
			if s, ok := v.Field(i).Interface().(string); ok {
				out[name] = redactSecret(s)
			} else {
				out[name] = "*****"
			}
			continue
		}

		out[name] = redactedValue(v.Field(i))
	}
	return out, nil
}

// redactedValue recurses into nested structs so their sensitive tags apply;
// anything else (including types with custom JSON marshaling, e.g. time.Time)
// passes through as-is.
func redactedValue(fv reflect.Value) any {
	deref := fv
	if deref.Kind() == reflect.Ptr {
		if deref.IsNil() {
			return nil
		}
		deref = deref.Elem()
	}
	if deref.Kind() == reflect.Struct {
		if _, isMarshaler := fv.Interface().(json.Marshaler); !isMarshaler {
			nested, err := redactedFields(deref)
			if err == nil {
				return nested
			}
		}
	}
	return fv.Interface()
}
