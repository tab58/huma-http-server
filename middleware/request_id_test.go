package middleware_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tab58/huma-http-server/middleware"
	"github.com/tab58/huma-http-server/router"
)

type reqIDOutput struct {
	Body struct {
		RequestID string `json:"request_id"`
	}
}

func newRequestIDRouter(t *testing.T) *router.Router[router.MapAuthInfo] {
	t.Helper()
	r := router.New(huma.Config{
		OpenAPI: &huma.OpenAPI{
			OpenAPI: "3.1.0",
			Info:    &huma.Info{Title: "test", Version: "0.0.1"},
		},
		Formats:       huma.DefaultFormats,
		DefaultFormat: "application/json",
	}, router.MapAuthInfoBuilder, router.WithMiddleware(middleware.RequestID()))

	router.RegisterRoute(r, router.RegisterRouteArgs[struct{}, reqIDOutput, router.MapAuthInfo]{
		Operation: huma.Operation{
			OperationID: "echo-reqid",
			Method:      http.MethodGet,
			Path:        "/reqid",
		},
		Handler: func(ctx context.Context, authInfo router.MapAuthInfo, input *struct{}) (*reqIDOutput, error) {
			out := &reqIDOutput{}
			out.Body.RequestID = middleware.GetRequestIDFromContext(ctx)
			return out, nil
		},
	})
	return r
}

func TestRequestIDGeneratedAndEchoedInResponse(t *testing.T) {
	r := newRequestIDRouter(t)

	w := get(r, "/reqid", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want %d", w.Code, http.StatusOK)
	}

	headerID := w.Header().Get("X-Request-Id")
	if headerID == "" {
		t.Fatal("response missing X-Request-Id header")
	}
	if !strings.Contains(w.Body.String(), headerID) {
		t.Fatalf("handler saw a different request ID than the response header %q: %s", headerID, w.Body.String())
	}
}

func TestInboundRequestIDPreserved(t *testing.T) {
	r := newRequestIDRouter(t)

	w := get(r, "/reqid", map[string]string{"X-Request-Id": "caller-supplied-id"})
	if got := w.Header().Get("X-Request-Id"); got != "caller-supplied-id" {
		t.Fatalf("X-Request-Id = %q, want caller-supplied-id echoed back", got)
	}
	if !strings.Contains(w.Body.String(), "caller-supplied-id") {
		t.Fatalf("handler did not see the caller-supplied ID: %s", w.Body.String())
	}
}
