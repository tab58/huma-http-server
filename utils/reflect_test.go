package utils

import "testing"

func TestIsStructOrStructPtr(t *testing.T) {
	type sample struct{ A int }

	tests := []struct {
		name     string
		in       any
		expected bool
	}{
		{"struct", sample{}, true},
		{"struct pointer", &sample{}, true},
		{"string", "nope", false},
		{"int pointer", new(int), false},
		{"map", map[string]string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsStructOrStructPtr(tt.in); got != tt.expected {
				t.Errorf("IsStructOrStructPtr(%T) = %v, want %v", tt.in, got, tt.expected)
			}
		})
	}
}
