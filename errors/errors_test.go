package errors

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestMapErrorToStatus(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{"bad request", ErrBadRequest, http.StatusBadRequest},
		{"unauthenticated", ErrUnauthenticated, http.StatusUnauthorized},
		{"unauthorized", ErrUnauthorized, http.StatusForbidden},
		{"not found", ErrNotFound, http.StatusNotFound},
		{"not implemented", ErrNotImplemented, http.StatusNotImplemented},
		{"internal", ErrInternalServerError, http.StatusInternalServerError},
		{"unknown defaults to 500", New("mystery"), http.StatusInternalServerError},
		{"wrapped sentinel still matches", fmt.Errorf("outer: %w", ErrNotFound), http.StatusNotFound},
		{"Wrap helper preserves sentinel", Wrap(ErrBadRequest, "context"), http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MapErrorToStatus(tt.err); got != tt.expected {
				t.Errorf("MapErrorToStatus(%v) = %d, want %d", tt.err, got, tt.expected)
			}
		})
	}
}

func TestMapErrorToHumaStatus(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{"bad request", ErrBadRequest, http.StatusBadRequest},
		{"unauthenticated", ErrUnauthenticated, http.StatusUnauthorized},
		{"unauthorized", ErrUnauthorized, http.StatusForbidden},
		{"not found", ErrNotFound, http.StatusNotFound},
		{"not implemented", ErrNotImplemented, http.StatusNotImplemented},
		{"internal", ErrInternalServerError, http.StatusInternalServerError},
		{"unknown defaults to 500", New("mystery"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MapErrorToHumaStatus(tt.err).GetStatus(); got != tt.expected {
				t.Errorf("MapErrorToHumaStatus(%v) status = %d, want %d", tt.err, got, tt.expected)
			}
		})
	}

	t.Run("5xx hides detail", func(t *testing.T) {
		// 4xx detail reaching the client is covered end-to-end in
		// router/register_test.go (TestNotFoundDetailPreserved)
		internal := MapErrorToHumaStatus(fmt.Errorf("secret detail: %w", ErrInternalServerError))
		if got := internal.Error(); got == "" || strings.Contains(got, "secret detail") {
			t.Errorf("5xx error message should be generic, got %q", got)
		}
	})
}
