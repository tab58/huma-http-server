package middleware

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CORSConfig configures the CORS middleware. AllowedOrigins is required —
// there is deliberately no allow-everything default.
type CORSConfig struct {
	// AllowedOrigins lists exact origins (scheme://host[:port]) allowed to
	// make cross-origin requests. The single entry "*" allows any origin.
	AllowedOrigins []string
	// AllowedMethods defaults to GET, POST, PUT, PATCH, DELETE, OPTIONS.
	AllowedMethods []string
	// AllowedHeaders defaults to Authorization, Content-Type, X-App-Key,
	// X-Request-Id.
	AllowedHeaders []string
	// AllowCredentials sets Access-Control-Allow-Credentials. With "*"
	// origins the request origin is echoed back, as the spec forbids the
	// wildcard together with credentials.
	AllowCredentials bool
	// MaxAge caps how long browsers may cache preflight results. Zero omits
	// the header.
	MaxAge time.Duration
}

var defaultCORSMethods = []string{
	http.MethodGet, http.MethodPost, http.MethodPut,
	http.MethodPatch, http.MethodDelete, http.MethodOptions,
}

var defaultCORSHeaders = []string{
	AUTHORIZATION_HEADER_NAME, "Content-Type", ACCESS_TOKEN_HEADER_NAME, "X-Request-Id",
}

// CORS wraps next with cross-origin response headers and preflight handling.
// It runs at the HTTP layer so preflight OPTIONS requests are answered even
// though no OPTIONS operation is registered. Disallowed origins get no CORS
// headers — the browser blocks the response.
func CORS(cfg CORSConfig, next http.Handler) http.Handler {
	methods := cfg.AllowedMethods
	if len(methods) == 0 {
		methods = defaultCORSMethods
	}
	headers := cfg.AllowedHeaders
	if len(headers) == 0 {
		headers = defaultCORSHeaders
	}
	allowMethods := strings.Join(methods, ", ")
	allowHeaders := strings.Join(headers, ", ")
	allowAny := slices.Contains(cfg.AllowedOrigins, "*")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// caches must not mix responses across origins
		w.Header().Add("Vary", "Origin")

		allowed := origin != "" && (allowAny || slices.Contains(cfg.AllowedOrigins, origin))
		if !allowed {
			next.ServeHTTP(w, r)
			return
		}

		if allowAny && !cfg.AllowCredentials {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		if cfg.AllowCredentials {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		// preflight: answer here, the mux has no OPTIONS routes
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Set("Access-Control-Allow-Methods", allowMethods)
			w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
			if cfg.MaxAge > 0 {
				w.Header().Set("Access-Control-Max-Age", strconv.Itoa(int(cfg.MaxAge.Seconds())))
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
