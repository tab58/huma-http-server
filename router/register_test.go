package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func okHandler[A AuthInfo](ctx context.Context, authInfo A, input *struct{}) (*testOutput, error) {
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

func testHumaConfig() huma.Config {
	return huma.Config{
		OpenAPI: &huma.OpenAPI{
			OpenAPI: "3.1.0",
			Info:    &huma.Info{Title: "test", Version: "0.0.1"},
		},
		Formats:       huma.DefaultFormats,
		DefaultFormat: "application/json",
	}
}

func newTestRouter(t *testing.T, opts ...RouterOption) *Router[MapAuthInfo] {
	t.Helper()
	return New(testHumaConfig(), MapAuthInfoBuilder, opts...)
}

func get[A AuthInfo](r *Router[A], path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.Mux().ServeHTTP(w, req)
	return w
}

func guardedRouteArgs(guards ...RouteGuardFunc[MapAuthInfo]) RegisterRouteArgs[struct{}, testOutput, MapAuthInfo] {
	return RegisterRouteArgs[struct{}, testOutput, MapAuthInfo]{
		Operation: huma.Operation{
			OperationID: "guarded",
			Method:      http.MethodGet,
			Path:        "/guarded",
		},
		Handler:     okHandler[MapAuthInfo],
		RouteGuards: guards,
	}
}

func TestGuardedRouteRejectsUnauthenticated(t *testing.T) {
	r := newTestRouter(t) // no auth middleware → authInfo is nil
	RegisterRoute(r, guardedRouteArgs(func(ctx context.Context, authInfo MapAuthInfo) error {
		return nil // guard would pass, but it must never run without auth info
	}))

	if w := get(r, "/guarded"); w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated request to guarded route: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestGuardedRouteAllowsPassingGuard(t *testing.T) {
	r := newTestRouter(t, WithMiddleware(withAuthInfo(map[string]string{"user_id": "u1"})))
	RegisterRoute(r, guardedRouteArgs(func(ctx context.Context, authInfo MapAuthInfo) error {
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
	RegisterRoute(r, guardedRouteArgs(func(ctx context.Context, authInfo MapAuthInfo) error {
		return errors.ErrUnauthorized
	}))

	if w := get(r, "/guarded"); w.Code != http.StatusForbidden {
		t.Fatalf("failing guard: got %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestUnguardedRouteAllowsAnonymous(t *testing.T) {
	r := newTestRouter(t)
	RegisterRoute(r, guardedRouteArgs()) // no guards

	if w := get(r, "/guarded"); w.Code != http.StatusOK {
		t.Fatalf("anonymous request to unguarded route: got %d, want %d", w.Code, http.StatusOK)
	}
}

type testUser struct {
	ID   string
	Role string
}

func (u testUser) UserID() string { return u.ID }

func buildTestUser(ctx context.Context, raw map[string]string) (testUser, error) {
	if raw["user_id"] == "" {
		return testUser{}, errors.Wrap(errors.ErrUnauthenticated, "missing user_id claim")
	}
	return testUser{ID: raw["user_id"], Role: raw["role"]}, nil
}

func TestServerWideTypedAuthInfo(t *testing.T) {
	r := New(testHumaConfig(), buildTestUser,
		WithMiddleware(withAuthInfo(map[string]string{"user_id": "u1", "role": "admin"})),
		WithMiddleware(middleware.WideEvent(middleware.WideEventConfig{ServiceName: "test"})))

	var handlerSaw testUser
	var eventUserID string
	RegisterRoute(r, RegisterRouteArgs[struct{}, testOutput, testUser]{
		Operation: huma.Operation{
			OperationID: "typed",
			Method:      http.MethodGet,
			Path:        "/typed",
		},
		Handler: func(ctx context.Context, authInfo testUser, input *struct{}) (*testOutput, error) {
			handlerSaw = authInfo
			if event := middleware.GetWideEventFromContext(ctx); event != nil {
				eventUserID = event.UserID
			}
			out := &testOutput{}
			out.Body.OK = true
			return out, nil
		},
		RouteGuards: []RouteGuardFunc[testUser]{
			func(ctx context.Context, u testUser) error {
				if u.Role != "admin" {
					return errors.ErrUnauthorized
				}
				return nil
			},
		},
	})

	if w := get(r, "/typed"); w.Code != http.StatusOK {
		t.Fatalf("typed auth route: got %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if handlerSaw.ID != "u1" || handlerSaw.Role != "admin" {
		t.Fatalf("handler saw %+v, want ID=u1 Role=admin", handlerSaw)
	}
	if eventUserID != "u1" {
		t.Fatalf("wide event UserID = %q, want %q (stamped via AuthInfo.UserID())", eventUserID, "u1")
	}
}

func TestAuthInfoBuilderErrorReturns401(t *testing.T) {
	r := New(testHumaConfig(), buildTestUser,
		WithMiddleware(withAuthInfo(map[string]string{"user_id": ""}))) // builder rejects empty user_id

	RegisterRoute(r, RegisterRouteArgs[struct{}, testOutput, testUser]{
		Operation: huma.Operation{
			OperationID: "badclaims",
			Method:      http.MethodGet,
			Path:        "/badclaims",
		},
		Handler: func(ctx context.Context, authInfo testUser, input *struct{}) (*testOutput, error) {
			t.Fatal("handler must not run when the builder fails")
			return nil, nil
		},
	})

	if w := get(r, "/badclaims"); w.Code != http.StatusUnauthorized {
		t.Fatalf("builder failure: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestInternalErrorDetailNotLeakedToClient(t *testing.T) {
	r := newTestRouter(t)
	RegisterRoute(r, RegisterRouteArgs[struct{}, testOutput, MapAuthInfo]{
		Operation: huma.Operation{
			OperationID: "boom",
			Method:      http.MethodGet,
			Path:        "/boom",
		},
		Handler: func(ctx context.Context, authInfo MapAuthInfo, input *struct{}) (*testOutput, error) {
			return nil, fmt.Errorf("db connection to 10.0.0.5 failed with password hunter2: %w", errors.ErrInternalServerError)
		},
	})

	w := get(r, "/boom")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("internal error: got %d, want %d", w.Code, http.StatusInternalServerError)
	}
	body := w.Body.String()
	if strings.Contains(body, "10.0.0.5") || strings.Contains(body, "hunter2") {
		t.Fatalf("internal error detail leaked to client: %s", body)
	}
}

func TestNotFoundDetailPreserved(t *testing.T) {
	// 4xx errors are intentional client messaging — detail stays
	r := newTestRouter(t)
	RegisterRoute(r, RegisterRouteArgs[struct{}, testOutput, MapAuthInfo]{
		Operation: huma.Operation{
			OperationID: "missing",
			Method:      http.MethodGet,
			Path:        "/missing",
		},
		Handler: func(ctx context.Context, authInfo MapAuthInfo, input *struct{}) (*testOutput, error) {
			return nil, fmt.Errorf("widget 42 does not exist: %w", errors.ErrNotFound)
		},
	})

	w := get(r, "/missing")
	if w.Code != http.StatusNotFound {
		t.Fatalf("not found: got %d, want %d", w.Code, http.StatusNotFound)
	}
	if !strings.Contains(w.Body.String(), "widget 42") {
		t.Fatalf("4xx detail should reach the client, got: %s", w.Body.String())
	}
}
