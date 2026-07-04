package server

import (
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tab58/huma-http-server/config"
	"github.com/tab58/huma-http-server/lib/jwt"
	"github.com/tab58/huma-http-server/middleware"
)

type ServerConfig struct {
	ServiceName        string
	ServiceVersion     string
	ServiceDescription string
	JWTSigningSecret   string
	Environment        config.AppMode
}

type serverConfigOptions struct {
	openAPIVersion    string
	openAPIPath       string
	docsPath          string
	schemasPath       string
	formats           map[string]huma.Format
	defaultFormat     string
	tokenGenerator    jwt.TokenGenerator
	idpPlugin         middleware.IdPPlugin
	middlewares       []func(ctx huma.Context, next func(huma.Context))
	skipPaths         []string
	readHeaderTimeout time.Duration
	readTimeout       time.Duration
	idleTimeout       time.Duration
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

		readHeaderTimeout: 5 * time.Second,
		readTimeout:       10 * time.Second,
		idleTimeout:       120 * time.Second,
	}
	for _, option := range options {
		option(&o)
	}
	return &o
}

type ServerConfigOption func(*serverConfigOptions)

func WithReadHeaderTimeout(timeout time.Duration) ServerConfigOption {
	return func(o *serverConfigOptions) {
		o.readHeaderTimeout = timeout
	}
}

func WithReadTimeout(timeout time.Duration) ServerConfigOption {
	return func(o *serverConfigOptions) {
		o.readTimeout = timeout
	}
}

func WithIdleTimeout(timeout time.Duration) ServerConfigOption {
	return func(o *serverConfigOptions) {
		o.idleTimeout = timeout
	}
}

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
