package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tab58/huma-http-server/middleware"
)

func corsHandler(cfg middleware.CORSConfig) http.Handler {
	return middleware.CORS(cfg, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func TestCORS(t *testing.T) {
	base := middleware.CORSConfig{AllowedOrigins: []string{"https://app.example.com"}}

	tests := []struct {
		name        string
		cfg         middleware.CORSConfig
		method      string
		headers     map[string]string
		wantCode    int
		wantOrigin  string
		wantMethods bool
	}{
		{
			name:       "allowed origin gets CORS headers",
			cfg:        base,
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": "https://app.example.com"},
			wantCode:   http.StatusOK,
			wantOrigin: "https://app.example.com",
		},
		{
			name:       "disallowed origin gets no CORS headers",
			cfg:        base,
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": "https://evil.example.com"},
			wantCode:   http.StatusOK,
			wantOrigin: "",
		},
		{
			name:       "same-origin request untouched",
			cfg:        base,
			method:     http.MethodGet,
			wantCode:   http.StatusOK,
			wantOrigin: "",
		},
		{
			name:   "preflight answered with 204",
			cfg:    middleware.CORSConfig{AllowedOrigins: []string{"https://app.example.com"}, MaxAge: 10 * time.Minute},
			method: http.MethodOptions,
			headers: map[string]string{
				"Origin":                        "https://app.example.com",
				"Access-Control-Request-Method": http.MethodPost,
			},
			wantCode:    http.StatusNoContent,
			wantOrigin:  "https://app.example.com",
			wantMethods: true,
		},
		{
			name:       "wildcard origin",
			cfg:        middleware.CORSConfig{AllowedOrigins: []string{"*"}},
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": "https://anything.example.com"},
			wantCode:   http.StatusOK,
			wantOrigin: "*",
		},
		{
			name:       "wildcard with credentials echoes origin",
			cfg:        middleware.CORSConfig{AllowedOrigins: []string{"*"}, AllowCredentials: true},
			method:     http.MethodGet,
			headers:    map[string]string{"Origin": "https://anything.example.com"},
			wantCode:   http.StatusOK,
			wantOrigin: "https://anything.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, "/x", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			corsHandler(tt.cfg).ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantCode)
			}
			if got := w.Header().Get("Access-Control-Allow-Origin"); got != tt.wantOrigin {
				t.Errorf("Allow-Origin = %q, want %q", got, tt.wantOrigin)
			}
			if tt.wantMethods {
				if w.Header().Get("Access-Control-Allow-Methods") == "" {
					t.Error("preflight missing Allow-Methods")
				}
				if w.Header().Get("Access-Control-Max-Age") != "600" {
					t.Errorf("Max-Age = %q, want 600", w.Header().Get("Access-Control-Max-Age"))
				}
			}
			if tt.cfg.AllowCredentials && tt.wantOrigin != "" {
				if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
					t.Error("missing Allow-Credentials")
				}
			}
		})
	}
}
