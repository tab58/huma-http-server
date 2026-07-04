package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tab58/huma-http-server/router"
)

func testServer() *Server[router.MapAuthInfo] {
	return New(ServerConfig{
		ServiceName:    "test",
		ServiceVersion: "0.0.1",
	}, router.MapAuthInfoBuilder)
}

type pingOutput struct {
	Body struct {
		UserID string `json:"user_id"`
	}
}

func TestServerRegisterRouteAndHandle(t *testing.T) {
	srv := New(ServerConfig{
		ServiceName:      "test",
		ServiceVersion:   "0.0.1",
		JWTSigningSecret: "test-secret",
	}, router.MapAuthInfoBuilder)

	// typed huma route through the root re-export
	RegisterRoute(srv, router.RegisterRouteArgs[struct{}, pingOutput, router.MapAuthInfo]{
		Operation: huma.Operation{OperationID: "ping", Method: http.MethodGet, Path: "/ping"},
		Handler: func(ctx context.Context, authInfo router.MapAuthInfo, input *struct{}) (*pingOutput, error) {
			out := &pingOutput{}
			out.Body.UserID = authInfo.UserID()
			return out, nil
		},
	})

	// raw handler through Handle
	srv.Handle("/raw", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	if srv.API() == nil {
		t.Fatal("API() returned nil")
	}

	mux := srv.router.Mux()

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ping", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("huma route: got %d, want 200: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/raw", nil))
	if w.Code != http.StatusTeapot {
		t.Fatalf("raw route: got %d, want 418", w.Code)
	}
}

func TestStartReturnsBindError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	if _, err := testServer().Start(ln.Addr().String()); err == nil {
		t.Fatal("expected bind error for occupied port, got nil")
	}
}

func TestStartAndGracefulShutdown(t *testing.T) {
	srv := testServer()
	errCh, err := srv.Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	select {
	case err, ok := <-errCh:
		if ok && err != nil {
			t.Fatalf("unexpected serve error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("error channel not closed after shutdown")
	}
}
