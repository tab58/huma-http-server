package server

import (
	"context"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tab58/huma-http-server/lib/jwt"
	"github.com/tab58/huma-http-server/middleware"
)

type fakeIdP struct{}

func (fakeIdP) ValidateAuthorizationHeader(ctx context.Context, authHeader string) (map[string]string, error) {
	return map[string]string{"user_id": "idp-user"}, nil
}

func TestAllServerConfigOptionsWired(t *testing.T) {
	gen := jwt.NewTokenGenerator("secret")
	mw := func(ctx huma.Context, next func(huma.Context)) { next(ctx) }
	formats := map[string]huma.Format{"application/json": huma.DefaultJSONFormat}

	opts := loadServerConfigOptions([]ServerConfigOption{
		WithOpenAPIVersion("3.0.3"),
		WithOpenAPIPath("/api-spec"),
		WithDocsPath("/api-docs"),
		WithSchemasPath("/api-schemas"),
		WithFormats(formats),
		WithDefaultFormat("application/json"),
		WithTokenGenerator(gen),
		WithMiddleware(mw),
		WithIdPPlugin(fakeIdP{}),
		WithSkipPaths([]string{"/healthz"}),
		WithReadHeaderTimeout(1 * time.Second),
		WithReadTimeout(2 * time.Second),
		WithIdleTimeout(3 * time.Second),
	})

	if opts.openAPIVersion != "3.0.3" || opts.openAPIPath != "/api-spec" ||
		opts.docsPath != "/api-docs" || opts.schemasPath != "/api-schemas" {
		t.Errorf("OpenAPI options not wired: %+v", opts)
	}
	if opts.defaultFormat != "application/json" || len(opts.formats) != 1 {
		t.Errorf("format options not wired: %+v", opts)
	}
	if opts.tokenGenerator == nil || opts.idpPlugin == nil {
		t.Error("tokenGenerator/idpPlugin not wired")
	}
	if len(opts.middlewares) != 1 {
		t.Errorf("middlewares = %d, want 1", len(opts.middlewares))
	}
	if len(opts.skipPaths) != 1 || opts.skipPaths[0] != "/healthz" {
		t.Errorf("skipPaths not wired: %v", opts.skipPaths)
	}
	if opts.readHeaderTimeout != 1*time.Second || opts.readTimeout != 2*time.Second || opts.idleTimeout != 3*time.Second {
		t.Errorf("timeouts not wired: %+v", opts)
	}
}

func TestWideEventOptionsWired(t *testing.T) {
	fn := func(e *middleware.WideEventContext) bool { return true }
	opts := loadServerConfigOptions([]ServerConfigOption{
		WithSampleRate(0.5),
		WithSlowThreshold(10 * time.Second),
		WithSampleFn(fn),
	})
	if opts.sampleRate != 0.5 {
		t.Errorf("sampleRate = %v, want 0.5", opts.sampleRate)
	}
	if opts.slowThreshold != 10*time.Second {
		t.Errorf("slowThreshold = %v, want 10s", opts.slowThreshold)
	}
	if opts.sampleFn == nil {
		t.Error("sampleFn not set")
	}
}
