package utils

import (
	"reflect"
)

func IsStructOrStructPtr(v any) bool {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct
}
