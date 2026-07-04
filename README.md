# huma-http-server

An opinionated HTTP API server library for Go, built on [Huma v2](https://github.com/danielgtaylor/huma). It wires up the boring-but-critical parts of a production API so services don't have to:

- **OpenAPI 3.1** docs served automatically (`/openapi`, `/docs`, `/schemas`)
- **JWT authentication** (HS256 access/refresh tokens) with optional external IdP support and refresh-token rotation/revocation
- **Typed routes** with a server-wide auth object and per-route guards
- **Wide-event structured logging** with tail sampling (errors and slow requests always kept)
- **Request IDs** injected and echoed back in the `X-Request-Id` response header
- **Safe defaults**: server timeouts (slowloris protection), generic 5xx messages (no internal detail leaks), secret-redacting config logging

## Requirements

Go 1.25+

## Installation

```bash
go get github.com/tab58/huma-http-server
```

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/danielgtaylor/huma/v2"
	server "github.com/tab58/huma-http-server"
	"github.com/tab58/huma-http-server/router"
)

type GreetingInput struct {
	Name string `path:"name" maxLength:"30" doc:"Name to greet"`
}

type GreetingOutput struct {
	Body struct {
		Message string `json:"message" doc:"The greeting"`
	}
}

func main() {
	// One AuthInfo type serves the whole server. MapAuthInfoBuilder is the
	// no-frills default: handlers receive the raw JWT claims as a map.
	srv := server.New(server.ServerConfig{
		ServiceName:    "greeter",
		ServiceVersion: "1.0.0",
	}, router.MapAuthInfoBuilder)

	server.RegisterRoute(srv, router.RegisterRouteArgs[GreetingInput, GreetingOutput, router.MapAuthInfo]{
		Operation: huma.Operation{
			OperationID: "get-greeting",
			Method:      http.MethodGet,
			Path:        "/greeting/{name}",
		},
		Handler: func(ctx context.Context, _ router.MapAuthInfo, input *GreetingInput) (*GreetingOutput, error) {
			out := &GreetingOutput{}
			out.Body.Message = fmt.Sprintf("Hello, %s!", input.Name)
			return out, nil
		},
	})

	errCh, err := srv.Start(":8888")
	if err != nil {
		fmt.Fprintln(os.Stderr, err) // e.g. port already in use
		os.Exit(1)
	}

	// wait for ctrl-c, then drain in-flight requests
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	select {
	case err := <-errCh:
		fmt.Fprintln(os.Stderr, "server error:", err)
	case <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}
}
```

Run it and visit `http://localhost:8888/docs` for the interactive API docs, or:

```bash
curl http://localhost:8888/greeting/world
# {"message":"Hello, world!"}
```

## Authentication

Set `JWTSigningSecret` to enable the JWT middleware. Access tokens are read from the `X-App-Key` header; an external IdP can additionally validate the `Authorization` header via `WithIdPPlugin`.

### A typed auth object

Define one auth type for the whole server. It must implement `router.AuthInfo` (`UserID() string`), and your builder converts verified JWT claims into it:

```go
type User struct {
	ID   string
	Role string
}

func (u User) UserID() string { return u.ID }

func BuildUser(ctx context.Context, claims map[string]string) (User, error) {
	if claims[router.USER_ID_CLAIM] == "" {
		return User{}, errors.Wrap(errors.ErrUnauthenticated, "token missing user_id claim")
	}
	return User{ID: claims["user_id"], Role: claims["role"]}, nil
}

srv := server.New(server.ServerConfig{
	ServiceName:      "greeter",
	ServiceVersion:   "1.0.0",
	JWTSigningSecret: os.Getenv("JWT_SIGNING_SECRET"),
}, BuildUser) // Server[User] — every handler and guard receives a User
```

### Route guards

Guards run after authentication, before the handler. A guarded route rejects unauthenticated requests with 401 automatically; guard errors return 403.

```go
adminOnly := func(ctx context.Context, u User) error {
	if u.Role != "admin" {
		return errors.ErrUnauthorized
	}
	return nil
}

server.RegisterRoute(srv, router.RegisterRouteArgs[In, Out, User]{
	Operation:   op,
	Handler:     handler,
	RouteGuards: []router.RouteGuardFunc[User]{adminOnly},
})
```

### Refresh-token rotation

Refresh tokens carry a `jti` claim. Plug in a shared denylist (Redis, DB) to revoke them; every exchange invalidates the old token:

```go
gen := jwt.NewTokenGeneratorWithRevocation(secret, myRevocationStore)
srv := server.New(cfg, BuildUser, server.WithTokenGenerator(gen))
```

Refresh tokens never authenticate requests — they are only accepted by your token-exchange endpoint via `ExchangeRefreshToken`.

## Raw routes

For static pages, file servers, or anything that isn't a typed API endpoint:

```go
srv.Handle("/", http.FileServer(http.Dir("./static")))
```

Raw routes bypass the middleware chain (request ID, auth, wide events) and don't appear in the OpenAPI spec. Register before `Start`.

## Error handling

Handlers return errors wrapped around the sentinels in the `errors` package; the router maps them to HTTP status codes:

```go
return nil, fmt.Errorf("order %s: %w", id, errors.ErrNotFound) // → 404, detail sent to client
return nil, fmt.Errorf("db down: %w", errors.ErrInternalServerError) // → 500, generic message; detail logged only
```

## Configuration

`server.New` accepts functional options: OpenAPI paths (`WithOpenAPIPath`, `WithDocsPath`, `WithSchemasPath`), server timeouts (`WithReadHeaderTimeout`, `WithReadTimeout`, `WithIdleTimeout`), wide-event sampling (`WithSampleRate`, `WithSlowThreshold`, `WithSampleFn`), auth (`WithTokenGenerator`, `WithIdPPlugin`), and more — see `config.go`.

App configuration loads from environment variables (and optionally a config file) into your own struct:

```go
type Config struct {
	Port      string `mapstructure:"PORT"`
	JWTSecret string `mapstructure:"JWT_SIGNING_SECRET" sensitive:"true"`
}

var cfg Config
if err := config.Load(&cfg); err != nil { // config.Load(&cfg, config.WithConfigFile("app.yaml"))
	log.Fatal(err)
}
```

Fields tagged `sensitive:"true"` are redacted in the startup log (only the last 5 characters shown).

## Observability

Every request gets a **wide event**: one structured JSON log line (via `log/slog`) with service metadata, method/path, status, duration, user ID, and error detail. Tail sampling keeps volume down — errors and slow requests are always logged, the rest at `SampleRate` (default 5%). Attach your own context from handlers:

```go
if event := middleware.GetWideEventFromContext(ctx); event != nil {
	event.AttachEventContext("db_query", map[string]string{"table": "orders"})
}
```

## Development

```bash
go build ./...
go vet ./...
go test ./... -race -cover   # every package ≥80% coverage
```

See `AGENTS.md` for the architecture overview and layout.
