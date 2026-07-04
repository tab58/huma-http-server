package server

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tab58/huma-http-server/config"
	"github.com/tab58/huma-http-server/lib/jwt"
	"github.com/tab58/huma-http-server/middleware"
	"github.com/tab58/huma-http-server/router"
	"github.com/tab58/huma-http-server/utils"
)

type Server struct {
	api huma.API
	mux *http.ServeMux
	srv *http.Server
}

// Start starts the server in a background goroutine.
// Call Shutdown to gracefully stop it.
func (s *Server) Start(addr string) {
	s.srv.Addr = addr
	go func() {
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		}
	}()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func (s *Server) API() huma.API {
	return s.api
}

type ServerConfig struct {
	ServiceName        string
	ServiceVersion     string
	ServiceDescription string
	JWTSigningSecret   string
	Environment        config.AppMode
}

type serverConfigOptions struct {
	openAPIVersion string
	openAPIPath    string
	docsPath       string
	schemasPath    string
	formats        map[string]huma.Format
	defaultFormat  string
	tokenGenerator jwt.TokenGenerator
	idpPlugin      middleware.IdPPlugin
	middlewares    []func(ctx huma.Context, next func(huma.Context))
	skipPaths      []string
}

type ServerConfigOption func(*serverConfigOptions)

func WithOpenAPIVersion(version string) ServerConfigOption {
	return func(o *serverConfigOptions) {
		o.openAPIVersion = version
	}
}

func WithOpenAPIPath(path string) ServerConfigOption {
	return func(o *serverConfigOptions) {
		o.openAPIPath = path
	}
}

func WithDocsPath(path string) ServerConfigOption {
	return func(o *serverConfigOptions) {
		o.docsPath = path
	}
}

func WithSchemasPath(path string) ServerConfigOption {
	return func(o *serverConfigOptions) {
		o.schemasPath = path
	}
}

func WithFormats(formats map[string]huma.Format) ServerConfigOption {
	return func(o *serverConfigOptions) {
		o.formats = formats
	}
}

func WithDefaultFormat(format string) ServerConfigOption {
	return func(o *serverConfigOptions) {
		o.defaultFormat = format
	}
}

func WithTokenGenerator(tokenGenerator jwt.TokenGenerator) ServerConfigOption {
	return func(o *serverConfigOptions) {
		o.tokenGenerator = tokenGenerator
	}
}

func WithMiddleware(middleware func(ctx huma.Context, next func(huma.Context))) ServerConfigOption {
	return func(o *serverConfigOptions) {
		o.middlewares = append(o.middlewares, middleware)
	}
}

func WithIdPPlugin(idpPlugin middleware.IdPPlugin) ServerConfigOption {
	return func(o *serverConfigOptions) {
		o.idpPlugin = idpPlugin
	}
}

func WithSkipPaths(skipPaths []string) ServerConfigOption {
	return func(o *serverConfigOptions) {
		o.skipPaths = skipPaths
	}
}

func loadServerConfigOptions(options []ServerConfigOption) *serverConfigOptions {
	o := serverConfigOptions{
		openAPIVersion: "3.1.0",
		openAPIPath:    "/openapi",
		docsPath:       "/docs",
		schemasPath:    "/schemas",
		formats:        huma.DefaultFormats,
		defaultFormat:  "application/json",
		middlewares:    make([]func(ctx huma.Context, next func(huma.Context)), 0),
		idpPlugin:      nil,
	}
	for _, option := range options {
		option(&o)
	}
	return &o
}

// New creates a new Huma API server.
func New(cfg ServerConfig, options ...ServerConfigOption) *Server {
	opts := loadServerConfigOptions(options)

	// build config objects
	var authenticator *middleware.Authenticator
	if cfg.JWTSigningSecret != "" {
		authenticator = &middleware.Authenticator{
			Generator:        jwt.NewTokenGenerator(cfg.JWTSigningSecret),
			IdentityProvider: opts.idpPlugin,
		}
	}
	wideEventConfig := middleware.WideEventConfig{
		ServiceName:    cfg.ServiceName,
		ServiceVersion: cfg.ServiceVersion,
		Environment:    string(cfg.Environment),
		SkipPaths:      buildSkipPaths(opts),
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
	router := router.New(serverConfig, routerOptions...)

	return &Server{
		srv: &http.Server{Handler: router.Mux()},
		api: router.API(),
		mux: router.Mux(),
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
	return utils.Keys(uniqueSkipPaths)
}

func buildRouterOptions(middlewares []func(ctx huma.Context, next func(huma.Context))) []router.RouterOption {
	routerOptions := make([]router.RouterOption, 0)
	for _, middleware := range middlewares {
		routerOptions = append(routerOptions, router.WithMiddleware(middleware))
	}
	return routerOptions
}
