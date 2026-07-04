package server

import (
	"context"
	"net"
	"testing"
	"time"
)

func testServer() *Server {
	return New(ServerConfig{
		ServiceName:    "test",
		ServiceVersion: "0.0.1",
	})
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
