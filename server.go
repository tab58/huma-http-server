package server

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"slices"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tab58/huma-http-server/lib/jwt"
	"github.com/tab58/huma-http-server/middleware"
	"github.com/tab58/huma-http-server/router"
)

// Server hosts a huma API with one server-wide AuthInfo type A, built from
// JWT claims by the AuthInfoBuilder passed to New.
type Server[A router.AuthInfo] struct {
	srv    *http.Server
	router *router.Router[A] // carries the API, mux, and AuthInfoBuilder
}

// Start binds to addr and serves in a background goroutine. Bind failures
// (e.g. port already in use) are returned immediately. Errors that occur
// while serving are sent on the returned channel, which is closed when the
// server stops; graceful Shutdown closes it without an error.
func (s *Server[A]) Start(addr string) (<-chan error, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	return errCh, nil
}

func (s *Server[A]) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func (s *Server[A]) API() huma.API {
	return s.router.API()
}

// Handle registers a raw http.Handler on the underlying mux (e.g. static
// pages, file servers, websocket upgrades). These routes bypass the huma
// middleware chain (request ID, auth, wide events) and do not appear in the
// OpenAPI spec. Register routes before calling Start.
func (s *Server[A]) Handle(pattern string, handler http.Handler) {
	s.router.Mux().Handle(pattern, handler)
}

// New creates a new Huma API server. The builder defines the single
// server-wide AuthInfo type: it converts verified JWT claims into A for
// every route. Servers without typed auth pass router.MapAuthInfoBuilder.
func New[A router.AuthInfo](cfg ServerConfig, builder router.AuthInfoBuilder[A], options ...ServerConfigOption) *Server[A] {
	opts := loadServerConfigOptions(options)

	// build config objects
	var authenticator *middleware.Authenticator
	if cfg.JWTSigningSecret != "" {
		// WithTokenGenerator overrides the default (e.g. to enable
		// refresh-token revocation via jwt.NewTokenGeneratorWithRevocation)
		generator := opts.tokenGenerator
		if generator == nil && cfg.JWTSigningSecret != "" {
			generator = jwt.NewTokenGenerator(cfg.JWTSigningSecret)
		}
		authenticator = &middleware.Authenticator{
			Generator:        generator,
			IdentityProvider: opts.idpPlugin,
		}
	}
	wideEventConfig := middleware.WideEventConfig{
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		Environment:    string(cfg.Environment),
		SkipPaths:      buildSkipPaths(opts),
		SampleRate:     opts.sampleRate,
		SlowThreshold:  opts.slowThreshold,
		SampleFn:       opts.sampleFn,
	}
	serverConfig := huma.Config{
		OpenAPI: &huma.OpenAPI{
			OpenAPI: opts.openAPIVersion,
			Info: &huma.Info{
				Title:       cfg.ServiceName,
				Version:     cfg.ServiceVersion,
				Description: cfg.ServiceDescription,
			},
		},
		OpenAPIPath:   opts.openAPIPath,
		DocsPath:      opts.docsPath,
		SchemasPath:   opts.schemasPath,
		Formats:       opts.formats,
		DefaultFormat: opts.defaultFormat,
	}

	// build the middlewares
	middlewares := []func(ctx huma.Context, next func(huma.Context)){
		middleware.RequestID(),
	}
	if authenticator != nil {
		middlewares = append(middlewares, middleware.Authentication(*authenticator))
	}
	middlewares = append(middlewares, middleware.WideEvent(wideEventConfig))
	middlewares = append(middlewares, opts.middlewares...)

	// build the router
	routerOptions := buildRouterOptions(middlewares)
	rtr := router.New(serverConfig, builder, routerOptions...)

	return &Server[A]{
		router: rtr,
		srv: &http.Server{
			Handler: rtr.Mux(),
			// ReadHeaderTimeout is the amount of time allowed to read request headers.
			// This is a security measure to prevent slowloris attacks.
			ReadHeaderTimeout: opts.readHeaderTimeout,
			// ReadTimeout is the maximum duration for reading the entire request, including the body.
			ReadTimeout: opts.readTimeout,
			// IdleTimeout is the maximum amount of time to wait for the next request when keep-alives are enabled.
			IdleTimeout: opts.idleTimeout,
		},
	}
}

func buildSkipPaths(opts *serverConfigOptions) []string {
	uniqueSkipPaths := make(map[string]any)
	uniqueSkipPaths[opts.openAPIPath] = struct{}{}
	uniqueSkipPaths[opts.docsPath] = struct{}{}
	uniqueSkipPaths[opts.schemasPath] = struct{}{}
	for _, path := range opts.skipPaths {
		uniqueSkipPaths[path] = struct{}{}
	}
	return slices.Collect(maps.Keys(uniqueSkipPaths))
}

func buildRouterOptions(middlewares []func(ctx huma.Context, next func(huma.Context))) []router.RouterOption {
	routerOptions := make([]router.RouterOption, 0)
	for _, middleware := range middlewares {
		routerOptions = append(routerOptions, router.WithMiddleware(middleware))
	}
	return routerOptions
}
