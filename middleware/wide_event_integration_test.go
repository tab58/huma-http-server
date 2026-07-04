package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

// capturedEvent decodes the "event" attr of a JSONHandler wide-event record.
type capturedEvent struct {
	Event struct {
		StatusCode int    `json:"status_code"`
		Method     string `json:"method"`
		Path       string `json:"path"`
		RequestID  string `json:"request_id"`
	} `json:"event"`
}

func decodeWideEvent(t *testing.T, buf *bytes.Buffer) capturedEvent {
	t.Helper()
	var rec capturedEvent
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("decode wide event log %q: %v", buf.String(), err)
	}
	return rec
}

func TestValidationFailureWideEventCarriesRequestIdentity(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	r := router.New(huma.Config{
		OpenAPI: &huma.OpenAPI{
			OpenAPI: "3.1.0",
			Info:    &huma.Info{Title: "test", Version: "0.0.1"},
		},
		Formats:       huma.DefaultFormats,
		DefaultFormat: "application/json",
	}, router.MapAuthInfoBuilder,
		router.WithMiddleware(middleware.RequestID()),
		router.WithMiddleware(middleware.WideEvent(middleware.WideEventConfig{ServiceName: "svc", Logger: logger})),
	)

	type validateInput struct {
		Body struct {
			Name string `json:"name" minLength:"3"`
		}
	}
	router.RegisterRoute(r, router.RegisterRouteArgs[validateInput, wideEventOutput, router.MapAuthInfo]{
		Operation: huma.Operation{OperationID: "validate", Method: http.MethodPost, Path: "/validate"},
		Handler: func(ctx context.Context, authInfo router.MapAuthInfo, input *validateInput) (*wideEventOutput, error) {
			return &wideEventOutput{}, nil
		},
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/validate", strings.NewReader(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.Mux().ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("got %d, want 422: %s", w.Code, w.Body.String())
	}

	// 4xx counts as an error → always sampled, so exactly this event is logged
	rec := decodeWideEvent(t, &buf)
	if rec.Event.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("event status_code = %d, want 422", rec.Event.StatusCode)
	}
	if rec.Event.Method != http.MethodPost || rec.Event.Path != "/validate" {
		t.Errorf("event method/path = %q %q, want POST /validate", rec.Event.Method, rec.Event.Path)
	}
	if rec.Event.RequestID == "" {
		t.Error("event request_id is empty")
	}
}

func TestUnmatchedRouteLogsWideEvent(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /known", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.WideEventNotFound(middleware.WideEventConfig{ServiceName: "svc", Logger: logger}, mux)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", w.Code)
	}
	rec := decodeWideEvent(t, &buf)
	if rec.Event.StatusCode != http.StatusNotFound || rec.Event.Path != "/nope" || rec.Event.Method != http.MethodGet {
		t.Errorf("event = %+v, want 404 GET /nope", rec.Event)
	}

	// matched routes are the huma middleware's job — the wrapper stays silent
	buf.Reset()
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/known", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200", w.Code)
	}
	if buf.Len() != 0 {
		t.Errorf("matched route logged by the 404 wrapper: %s", buf.String())
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
