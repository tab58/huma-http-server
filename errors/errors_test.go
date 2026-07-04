package errors

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
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
		{"conflict", ErrConflict, http.StatusConflict},
		{"too many requests", ErrTooManyRequests, http.StatusTooManyRequests},
		{"not implemented", ErrNotImplemented, http.StatusNotImplemented},
		{"internal", ErrInternalServerError, http.StatusInternalServerError},
		{"unknown defaults to 500", New("mystery"), http.StatusInternalServerError},
		{"wrapped sentinel still matches", fmt.Errorf("outer: %w", ErrNotFound), http.StatusNotFound},
		{"Wrap helper preserves sentinel", Wrap(ErrBadRequest, "context"), http.StatusBadRequest},
		{"huma StatusError reports its own status", huma.Error409Conflict("already running"), http.StatusConflict},
		{"wrapped huma StatusError reports its own status", fmt.Errorf("outer: %w", huma.Error429TooManyRequests("slow down")), http.StatusTooManyRequests},
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
		{"conflict", ErrConflict, http.StatusConflict},
		{"too many requests", ErrTooManyRequests, http.StatusTooManyRequests},
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

	t.Run("existing StatusError passes through unchanged", func(t *testing.T) {
		orig := huma.Error409Conflict("already running")
		got := MapErrorToHumaStatus(orig)
		if got != orig {
			t.Errorf("MapErrorToHumaStatus should return the original StatusError, got %v", got)
		}
		wrapped := MapErrorToHumaStatus(fmt.Errorf("outer: %w", orig))
		if wrapped != orig {
			t.Errorf("wrapped StatusError should unwrap to the original, got %v", wrapped)
		}
	})

	t.Run("5xx hides detail", func(t *testing.T) {
		// 4xx detail reaching the client is covered end-to-end in
		// router/register_test.go (TestNotFoundDetailPreserved)
		internal := MapErrorToHumaStatus(fmt.Errorf("secret detail: %w", ErrInternalServerError))
		if got := internal.Error(); got == "" || strings.Contains(got, "secret detail") {
			t.Errorf("5xx error message should be generic, got %q", got)
		}
	})
}
