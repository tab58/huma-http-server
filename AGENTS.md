# AGENTS.md

Root documentation for this repository. Source of truth for layout, behavior, and conventions (see `CLAUDE.md` for the doc-maintenance rules). Update this file in the same change whenever code alters anything described here.

## What this is

A reusable Go library (module `github.com/tab58/huma-http-server`, Go 1.25) that wraps [Huma v2](https://github.com/danielgtaylor/huma) into an opinionated HTTP API server with batteries included:

- OpenAPI 3.1 docs/schemas served automatically (`/openapi`, `/docs`, `/schemas`)
- JWT access/refresh token auth (HS256) with an optional external IdP plugin
- Request-ID injection
- "Wide event" structured request logging with tail sampling
- Typed route registration with per-route guard functions and domain-error → HTTP-status mapping

It is a library, not an application: there is no `main`, no config file, no Dockerfile. Consumers import `server` (root package), call `server.New(...)`, register routes via `router.RegisterRoute`, then `Start`/`Shutdown`.

## Build / test

No Taskfile/Makefile/CI. Plain Go toolchain:

```bash
go build ./...
go vet ./...
go test ./...                 # only lib/jwt has tests today
go test ./lib/jwt -run TestAccessTokenRoundTrip   # single test
go test ./... -cover -race
```

## Layout

| Path | Package | Purpose |
|------|---------|---------|
| `server.go` | `server` | Entry point. `Server` struct, `New()` wires middleware + router, `Start(addr) (<-chan error, error)` binds synchronously then serves in a goroutine (serve errors on the channel; closed on stop); `Shutdown` for graceful stop. `Handle(pattern, handler)` mounts raw `http.Handler` routes (bypass huma middlewares and OpenAPI; register before `Start`). HTTP server timeouts set from options (defaults: read-header 5 s, read 10 s, idle 120 s; no write timeout). |
| `config.go` | `server` | `ServerConfig`, `serverConfigOptions`, and all `With*` functional options (OpenAPI paths, formats, token generator, IdP plugin, middlewares, skip paths, timeouts). |
| `register.go` | `server` | Convenience re-export of `router.RegisterRoute` so consumers only import the root package. |
| `router/router.go` | `router` | Thin wrapper: `http.ServeMux` + `humago` adapter, attaches middlewares via `api.UseMiddleware`. |
| `router/register.go` | `router` | `RegisterRoute[I, O, AuthInfo]` — generic route registration. Pulls request ID / auth info / wide event from context, runs route guards, maps handler errors to `huma.StatusError`. |
| `middleware/authentication.go` | `middleware` | Auth middleware. Checks `X-App-Key` header (access token), then `Authorization` header via `IdPPlugin`. Refresh tokens never authenticate requests — they are only for the token-exchange endpoint (`ExchangeRefreshToken`). Puts `map[string]string` auth info in context. Verification failure continues unauthenticated but records the error in context (`GetAuthErrorFromContext`); enforcement happens at the route layer. |
| `middleware/request_id.go` | `middleware` | Request-ID middleware (`X-Request-Id` honored if present, else `hostname/base62-counter`). |
| `middleware/wide_event.go` | `middleware` | Per-request `WideEventContext` (service metadata, timing, status, error), logged via `slog` as JSON. Tail sampling: always keep errors and slow requests, otherwise `SampleFn`/`SampleRate`. |
| `lib/jwt/` | `jwt` | HS256 JWT create/verify. `AccessToken`/`RefreshToken` string types, 15 min / 7 day expiries, `TokenGenerator` interface + impl, refresh-token exchange. Claims are flat `map[string]string` plus reserved `exp`/`iat`/`jti`/`typ` (reserved names in caller data are dropped). Every token carries a `typ` claim (`access`/`refresh`) verified with an exact, fail-closed match; the `exp - iat` duration check (±1 s) remains as claim sanity. Refresh tokens carry a random `jti`; `RevocationStore` + `NewTokenGeneratorWithRevocation` enable rotation/denylist (verify fails closed, exchange revokes the old jti). Tests: `jwt_test.go`, `rotation_test.go`, `typ_test.go`. |
| `errors/errors.go` | `errors` | Domain sentinel errors (`ErrBadRequest`, `ErrUnauthenticated`, `ErrUnauthorized`, `ErrNotFound`, `ErrNotImplemented`, `ErrInternalServerError`), re-exported `Is`/`As`/`New`/`Wrap`, and `MapErrorToStatus`/`MapErrorToHumaStatus`. |
| `config/load.go` | `config` | Generic `Load[T]` via viper: binds env vars from `mapstructure` tags, reads optional config file, unmarshals into caller's struct. `AppMode` (`development`/`production`). Uses the global viper instance. |
| `utils/` | `utils` | Small generic helpers (`Keys`, `Dedupe`, `Map`, `Filter`, `IsStructOrStructPtr`, …). Several duplicate modern stdlib (`maps`/`slices`) — candidates for deletion, see TODOs. |
| `TODO_ITEMS.md` | — | Production-readiness review findings. Read before changing auth, server lifecycle, or error handling. |

## How a request flows

1. `http.ServeMux` → `humago` adapter → huma middleware chain, in this order: **RequestID → Authentication (only if `JWTSigningSecret` set) → WideEvent → caller middlewares** (`server.go:New`).
2. WideEvent skips `SkipPaths` (always includes the OpenAPI/docs/schemas paths plus `WithSkipPaths` extras).
3. `RegisterRoute`'s wrapper handler stamps request ID / method / path / user_id / auth_error onto the wide event, then enforces route guards: on a guarded route, nil auth info → 401 ("invalid credentials" if bad credentials were presented, else "authentication required"); any guard error → 403; otherwise it calls the typed handler `func(ctx, authInfo, *I) (*O, error)`. Auth failure detail goes to the wide event only, never the response body.
4. Handler errors are matched against `errors` sentinels with `errors.Is` and converted to the corresponding `huma.Error4xx/5xx`; unknown errors become 500.

## Conventions

- **Functional options everywhere**: `ServerConfigOption`, `RouterOption`, `RegisterOption` all follow the `With*(...)` + private `loadXxxOptions` pattern. New knobs should follow it too.
- **Domain errors, not status codes, in handlers**: handlers return errors wrapped around the `errors` package sentinels (`fmt.Errorf("...: %w", errors.ErrNotFound)`); the router maps them to HTTP.
- **Auth info is `map[string]string`** end to end (JWT claims, IdP plugin result, context value, guard input). `user_id` is the key read for wide-event attribution.
- **Context values** use unexported key types with `Get*FromContext` accessors that return zero values on absence — never panic.
- Naming inconsistency to be aware of: `lib/jwt` uses `SCREAMING_SNAKE` consts (`ACCESS_TOKEN_EXPIRY`), non-idiomatic Go; middleware headers likewise (`AUTHORIZATION_HEADER_NAME`). Match local style when editing those files.

## Current state / known issues

`TODO_ITEMS.md` is the authoritative list. Highlights that materially affect any work here:

- `config.Load` logs the full config (secrets included) and its config-file log branches are inverted.
- Wide-event `SampleRate`/`SlowThreshold` defaults are documented but never applied (zero values used), and they aren't reachable from `server.New`'s options.
- `router/register.go`'s `getErrorStatusCode`/`getHumaErrorStatus` duplicate `errors.MapErrorToStatus`/`MapErrorToHumaStatus`.
- Test coverage exists for `lib/jwt`, the root `server` package (`server_test.go`: Start/Shutdown lifecycle), `router` (`register_test.go`: guard enforcement), and `middleware` (`authentication_test.go`: auth error handling). Request-ID, wide-event, and config are untested.

When fixing any of these, check the corresponding `TODO_ITEMS.md` entry off and update this section.
