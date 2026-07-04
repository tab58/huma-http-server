package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tab58/huma-http-server/lib/jwt"
	"github.com/tab58/huma-http-server/middleware"
	"github.com/tab58/huma-http-server/router"
)

const testSecret = "test-signing-secret"

type testOutput struct {
	Body struct {
		AuthInfoPresent  bool `json:"auth_info_present"`
		AuthErrorPresent bool `json:"auth_error_present"`
	}
}

// echoHandler reports what auth state actually reached the handler.
func echoHandler(ctx context.Context, authInfo router.MapAuthInfo, input *struct{}) (*testOutput, error) {
	out := &testOutput{}
	out.Body.AuthInfoPresent = authInfo != nil
	out.Body.AuthErrorPresent = middleware.GetAuthErrorFromContext(ctx) != nil
	return out, nil
}

func newAuthRouter(t *testing.T) *router.Router[router.MapAuthInfo] {
	t.Helper()
	authenticator := middleware.Authenticator{Generator: jwt.NewTokenGenerator(testSecret)}
	return router.New(huma.Config{
		OpenAPI: &huma.OpenAPI{
			OpenAPI: "3.1.0",
			Info:    &huma.Info{Title: "test", Version: "0.0.1"},
		},
		Formats:       huma.DefaultFormats,
		DefaultFormat: "application/json",
	}, router.MapAuthInfoBuilder, router.WithMiddleware(middleware.Authentication(authenticator)))
}

func registerRoute(r *router.Router[router.MapAuthInfo], path string, guards ...router.RouteGuardFunc[router.MapAuthInfo]) {
	router.RegisterRoute(r, router.RegisterRouteArgs[struct{}, testOutput, router.MapAuthInfo]{
		Operation: huma.Operation{
			OperationID: strings.TrimPrefix(path, "/"),
			Method:      http.MethodGet,
			Path:        path,
		},
		Handler:     echoHandler,
		RouteGuards: guards,
	})
}

func get(r *router.Router[router.MapAuthInfo], path string, headers map[string]string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	r.Mux().ServeHTTP(w, req)
	return w
}

func allowAll(ctx context.Context, authInfo router.MapAuthInfo) error { return nil }

func validToken(t *testing.T) string {
	t.Helper()
	token, err := jwt.CreateAccessToken(context.Background(), map[string]string{"user_id": "u1"}, testSecret)
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	return string(token)
}

func TestInvalidTokenOnGuardedRouteReturns401(t *testing.T) {
	r := newAuthRouter(t)
	registerRoute(r, "/guarded", allowAll)

	w := get(r, "/guarded", map[string]string{middleware.ACCESS_TOKEN_HEADER_NAME: "garbage"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid token on guarded route: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(w.Body.String(), "invalid credentials") {
		t.Fatalf("expected 'invalid credentials' in body, got: %s", w.Body.String())
	}
}

func TestMissingTokenOnGuardedRouteReturns401(t *testing.T) {
	r := newAuthRouter(t)
	registerRoute(r, "/guarded", allowAll)

	w := get(r, "/guarded", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing token on guarded route: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(w.Body.String(), "authentication required") {
		t.Fatalf("expected 'authentication required' in body, got: %s", w.Body.String())
	}
}

func TestValidTokenOnGuardedRouteSucceeds(t *testing.T) {
	r := newAuthRouter(t)
	registerRoute(r, "/guarded", allowAll)

	w := get(r, "/guarded", map[string]string{middleware.ACCESS_TOKEN_HEADER_NAME: validToken(t)})
	if w.Code != http.StatusOK {
		t.Fatalf("valid token on guarded route: got %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"auth_info_present":true`) {
		t.Fatalf("expected auth info in handler, got: %s", w.Body.String())
	}
}

func TestRefreshTokenCookieDoesNotAuthenticate(t *testing.T) {
	r := newAuthRouter(t)
	registerRoute(r, "/guarded", allowAll)

	refreshToken, err := jwt.CreateRefreshToken(context.Background(), map[string]string{"user_id": "u1"}, testSecret)
	if err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_token", Value: string(refreshToken)})
	r.Mux().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("valid refresh token cookie on guarded route: got %d, want %d — refresh tokens must not act as request auth", w.Code, http.StatusUnauthorized)
	}
}

func TestInvalidTokenOnPublicRoutePassesThroughUnauthenticated(t *testing.T) {
	r := newAuthRouter(t)
	registerRoute(r, "/public") // no guards

	w := get(r, "/public", map[string]string{middleware.ACCESS_TOKEN_HEADER_NAME: "garbage"})
	if w.Code != http.StatusOK {
		t.Fatalf("invalid token on public route: got %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"auth_info_present":false`) {
		t.Fatalf("expected no auth info, got: %s", body)
	}
	if !strings.Contains(body, `"auth_error_present":true`) {
		t.Fatalf("expected auth error recorded in context, got: %s", body)
	}
}
