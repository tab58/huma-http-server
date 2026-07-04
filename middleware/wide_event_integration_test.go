package middleware_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tab58/huma-http-server/errors"
	"github.com/tab58/huma-http-server/lib/jwt"
	"github.com/tab58/huma-http-server/middleware"
	"github.com/tab58/huma-http-server/router"
)

type wideEventOutput struct {
	Body struct {
		HasEvent bool `json:"has_event"`
	}
}

func newWideEventRouter(t *testing.T, cfg middleware.WideEventConfig) *router.Router[router.MapAuthInfo] {
	t.Helper()
	return router.New(huma.Config{
		OpenAPI: &huma.OpenAPI{
			OpenAPI: "3.1.0",
			Info:    &huma.Info{Title: "test", Version: "0.0.1"},
		},
		Formats:       huma.DefaultFormats,
		DefaultFormat: "application/json",
	}, router.MapAuthInfoBuilder, router.WithMiddleware(middleware.WideEvent(cfg)))
}

func TestWideEventAttachedToRequests(t *testing.T) {
	r := newWideEventRouter(t, middleware.WideEventConfig{ServiceName: "svc", ServiceVersion: "1.0", Environment: "test"})

	router.RegisterRoute(r, router.RegisterRouteArgs[struct{}, wideEventOutput, router.MapAuthInfo]{
		Operation: huma.Operation{OperationID: "we", Method: http.MethodGet, Path: "/we"},
		Handler: func(ctx context.Context, authInfo router.MapAuthInfo, input *struct{}) (*wideEventOutput, error) {
			event := middleware.GetWideEventFromContext(ctx)
			out := &wideEventOutput{}
			out.Body.HasEvent = event != nil
			if event != nil {
				// exercise the event-context helpers
				event.AttachEventContext("db_query", map[string]string{"table": "users"})
				if event.ServiceName != "svc" || event.HasError() {
					t.Errorf("unexpected event state: %+v", event)
				}
			}
			return out, nil
		},
	})

	w := get(r, "/we", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, `"has_event":true`) {
		t.Fatalf("handler did not see a wide event: %s", body)
	}
}

func TestWideEventSkipsConfiguredPaths(t *testing.T) {
	r := newWideEventRouter(t, middleware.WideEventConfig{ServiceName: "svc", SkipPaths: []string{"/skipped"}})

	router.RegisterRoute(r, router.RegisterRouteArgs[struct{}, wideEventOutput, router.MapAuthInfo]{
		Operation: huma.Operation{OperationID: "skipped", Method: http.MethodGet, Path: "/skipped"},
		Handler: func(ctx context.Context, authInfo router.MapAuthInfo, input *struct{}) (*wideEventOutput, error) {
			out := &wideEventOutput{}
			out.Body.HasEvent = middleware.GetWideEventFromContext(ctx) != nil
			return out, nil
		},
	})

	w := get(r, "/skipped", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
	if strings.Contains(w.Body.String(), `"has_event":true`) {
		t.Fatalf("skip path still got a wide event: %s", w.Body.String())
	}
}

func TestWideEventLogsErrorRequests(t *testing.T) {
	// error responses always sample → exercises the logging path
	r := newWideEventRouter(t, middleware.WideEventConfig{ServiceName: "svc"})

	router.RegisterRoute(r, router.RegisterRouteArgs[struct{}, wideEventOutput, router.MapAuthInfo]{
		Operation: huma.Operation{OperationID: "fail", Method: http.MethodGet, Path: "/fail"},
		Handler: func(ctx context.Context, authInfo router.MapAuthInfo, input *struct{}) (*wideEventOutput, error) {
			return nil, errors.Wrap(errors.ErrInternalServerError, "boom")
		},
	})

	if w := get(r, "/fail", nil); w.Code != http.StatusInternalServerError {
		t.Fatalf("got %d, want 500", w.Code)
	}
}

func TestAuthenticatorDelegates(t *testing.T) {
	ctx := context.Background()
	a := middleware.Authenticator{Generator: jwt.NewTokenGenerator(testSecret)}

	access, refresh, err := a.GenerateNewTokenPair(ctx, map[string]string{"user_id": "u1"})
	if err != nil {
		t.Fatalf("GenerateNewTokenPair: %v", err)
	}
	if access == "" || refresh == "" {
		t.Fatal("empty token pair")
	}

	newAccess, newRefresh, err := a.ExchangeRefreshToken(ctx, refresh)
	if err != nil {
		t.Fatalf("ExchangeRefreshToken: %v", err)
	}
	if newAccess == "" || newRefresh == "" {
		t.Fatal("empty exchanged token pair")
	}
}
