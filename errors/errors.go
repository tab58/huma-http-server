package errors

import (
	errs "errors"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

var (
	// Not Implemented.
	// Should map to HTTP 501 Not Implemented.
	ErrNotImplemented = errs.New("not implemented")

	// Bad Request.
	// Should map to HTTP 400 Bad Request.
	// Semantically this means the request is invalid, meaning the request is malformed or contains invalid data.
	// This is different from ErrUnauthorized because this deals with the request, not the credentials.
	ErrBadRequest = errs.New("bad request")

	// Should map to HTTP 401 Unauthorized.
	// Semantically this means the user is not authenticated, meaning credentials are invalid.
	ErrUnauthenticated = errs.New("unauthorized")

	// Should map to HTTP 403 Forbidden.
	// Semantically this means the user is authenticated but not authorized to access the resource.
	ErrUnauthorized = errs.New("forbidden")

	// Not Found.
	// Should map to HTTP 404 Not Found.
	// Semantically this means the resource was not found.
	ErrNotFound = errs.New("not found")

	// Should map to HTTP 500 Internal Server Error.
	// Semantically this is a general error that is not specific to the request.
	ErrInternalServerError = errs.New("internal server error")
)

// Re-export standard errors functions for convenience.
var (
	Is   = errs.Is
	As   = errs.As
	New  = errs.New
	Wrap = func(err error, msg string) error {
		return fmt.Errorf("%s: %w", msg, err)
	}
)

// MapErrorToStatus maps a domain error to an HTTP status code.
func MapErrorToStatus(err error) int {
	switch {
	case errs.Is(err, ErrBadRequest):
		return http.StatusBadRequest
	case errs.Is(err, ErrUnauthenticated):
		return http.StatusUnauthorized
	case errs.Is(err, ErrUnauthorized):
		return http.StatusForbidden
	case errs.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errs.Is(err, ErrNotImplemented):
		return http.StatusNotImplemented
	case errs.Is(err, ErrInternalServerError):
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// MapErrorToHumaStatus maps a domain error to a huma.StatusError for HTTP
// responses. 4xx errors carry their detail to the client (intentional
// messaging); 5xx errors return a generic message only — the real error must
// be logged server-side (the wide event does this), never sent to clients.
func MapErrorToHumaStatus(err error) huma.StatusError {
	code := MapErrorToStatus(err)
	switch code {
	case http.StatusBadRequest:
		return huma.Error400BadRequest("", err)
	case http.StatusUnauthorized:
		return huma.Error401Unauthorized("", err)
	case http.StatusForbidden:
		return huma.Error403Forbidden("", err)
	case http.StatusNotFound:
		return huma.Error404NotFound("", err)
	case http.StatusNotImplemented:
		return huma.Error501NotImplemented("not implemented")
	default:
		return huma.Error500InternalServerError("internal server error")
	}
}
