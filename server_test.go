package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tab58/huma-http-server/middleware"
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

func TestWithCORSPreflight(t *testing.T) {
	srv := New(ServerConfig{
		ServiceName:    "test",
		ServiceVersion: "0.0.1",
	}, router.MapAuthInfoBuilder, WithCORS(middleware.CORSConfig{
		AllowedOrigins: []string{"https://app.example.com"},
	}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/anything", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	srv.srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("preflight: got %d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("Allow-Origin = %q", got)
	}
}

func TestUnmatchedRouteReturns404ThroughServerHandler(t *testing.T) {
	srv := testServer()
	w := httptest.NewRecorder()
	srv.srv.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/definitely-not-registered", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", w.Code)
	}
}

// selfSignedCert writes a throwaway localhost certificate and key, returning
// their paths.
func selfSignedCert(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("MarshalECPrivateKey: %v", err)
	}

	dir := t.TempDir()
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certFile, keyFile
}

func TestStartTLSServesHTTPS(t *testing.T) {
	certFile, keyFile := selfSignedCert(t)
	srv := testServer()

	// reserve a port, then hand its address to StartTLS
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	errCh, err := srv.StartTLS(addr, certFile, keyFile)
	if err != nil {
		t.Fatalf("StartTLS: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
		<-errCh
	}()

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	resp, err := client.Get("https://" + addr + "/openapi.json")
	if err != nil {
		t.Fatalf("HTTPS GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("got %d, want 200", resp.StatusCode)
	}
	if resp.TLS == nil {
		t.Fatal("response was not served over TLS")
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
