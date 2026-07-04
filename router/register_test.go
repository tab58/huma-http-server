package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tab58/huma-http-server/errors"
	"github.com/tab58/huma-http-server/middleware"
)

type testOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func okHandler(ctx context.Context, authInfo map[string]string, input *struct{}) (*testOutput, error) {
	out := &testOutput{}
	out.Body.OK = true
	return out, nil
}

// withAuthInfo simulates the authentication middleware having resolved a user.
func withAuthInfo(info map[string]string) func(ctx huma.Context, next func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithValue(ctx, middleware.AuthContextKey, info))
	}
}

func newTestRouter(t *testing.T, opts ...RouterOption) *Router {
	t.Helper()
	return New(huma.Config{
		OpenAPI: &huma.OpenAPI{
			OpenAPI: "3.1.0",
			Info:    &huma.Info{Title: "test", Version: "0.0.1"},
		},
		Formats:       huma.DefaultFormats,
		DefaultFormat: "application/json",
	}, opts...)
}

func get(r *Router, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.Mux().ServeHTTP(w, req)
	return w
}

func guardedRouteArgs(r *Router, guards ...RouteGuardFunc[map[string]string]) RegisterRouteArgs[struct{}, testOutput, map[string]string] {
	return RegisterRouteArgs[struct{}, testOutput, map[string]string]{
		API: r.API(),
		Operation: huma.Operation{
			OperationID: "guarded",
			Method:      http.MethodGet,
			Path:        "/guarded",
		},
		Handler:     okHandler,
		RouteGuards: guards,
	}
}

func TestGuardedRouteRejectsUnauthenticated(t *testing.T) {
	r := newTestRouter(t) // no auth middleware → authInfo is nil
	RegisterRoute(guardedRouteArgs(r, func(ctx context.Context, authInfo map[string]string) error {
		return nil // guard would pass, but it must never run without auth info
	}))

	if w := get(r, "/guarded"); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request to guarded route: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestGuardedRouteAllowsPassingGuard(t *testing.T) {
	r := newTestRouter(t, WithMiddleware(withAuthInfo(map[string]string{"user_id": "u1"})))
	RegisterRoute(guardedRouteArgs(r, func(ctx context.Context, authInfo map[string]string) error {
		if authInfo["user_id"] != "u1" {
			return errors.ErrUnauthorized
		}
		return nil
	}))

	if w := get(r, "/guarded"); w.Code != http.StatusOK {
		t.Fatalf("authenticated request passing guard: got %d, want %d", w.Code, http.StatusOK)
	}
}

func TestGuardedRouteRejectsFailingGuard(t *testing.T) {
	r := newTestRouter(t, WithMiddleware(withAuthInfo(map[string]string{"user_id": "intruder"})))
	RegisterRoute(guardedRouteArgs(r, func(ctx context.Context, authInfo map[string]string) error {
		return errors.ErrUnauthorized
	}))

	if w := get(r, "/guarded"); w.Code != http.StatusForbidden {
		t.Fatalf("failing guard: got %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestUnguardedRouteAllowsAnonymous(t *testing.T) {
	r := newTestRouter(t)
	RegisterRoute(guardedRouteArgs(r)) // no guards

	if w := get(r, "/guarded"); w.Code != http.StatusOK {
		t.Fatalf("anonymous request to unguarded route: got %d, want %d", w.Code, http.StatusOK)
	}
}
