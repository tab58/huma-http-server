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
| `server.go` | `server` | Entry point. `Server[A router.AuthInfo]`; `New[A](cfg, builder, options...)` wires middleware + router with the server-wide auth type, `Start(addr) (<-chan error, error)` binds synchronously then serves in a goroutine (serve errors on the channel; closed on stop); `Shutdown` for graceful stop. `Handle(pattern, handler)` mounts raw `http.Handler` routes (bypass huma middlewares and OpenAPI; register before `Start`). HTTP server timeouts set from options (defaults: read-header 5 s, read 10 s, idle 120 s; no write timeout). |
| `config.go` | `server` | `ServerConfig`, `serverConfigOptions`, and all `With*` functional options (OpenAPI paths, formats, token generator, IdP plugin, middlewares, skip paths, timeouts). |
| `register.go` | `server` | Convenience re-export: `RegisterRoute(s *Server[A], args, opts...)` delegates to `router.RegisterRoute` so consumers only import the root package. |
| `router/router.go` | `router` | `Router[A AuthInfo]`: `http.ServeMux` + `humago` adapter + the server-wide `AuthInfoBuilder[A]`; middlewares attach via `api.UseMiddleware`. |
| `router/auth_info.go` | `router` | `AuthInfo` interface (`UserID() string`) — the contract every server-wide auth type implements. `AuthInfoBuilder[A]` (claims → `A`), `MapAuthInfo` (raw-map pass-through, `UserID` from the `user_id` claim / `USER_ID_CLAIM`), `MapAuthInfoBuilder`. |
| `router/register.go` | `router` | `RegisterRoute[I, O any, A AuthInfo](r *Router[A], args, opts...)` — typed route registration. Pulls request ID / auth info / wide event from context, converts raw claims to `A` via the router's builder (error → 401), stamps `event.UserID` from `A.UserID()`, runs route guards, maps handler errors to `huma.StatusError`. |
| `middleware/authentication.go` | `middleware` | Auth middleware. Checks `X-App-Key` header (access token), then `Authorization` header via `IdPPlugin`. Refresh tokens never authenticate requests — they are only for the token-exchange endpoint (`ExchangeRefreshToken`). Puts `map[string]string` auth info in context. Verification failure continues unauthenticated but records the error in context (`GetAuthErrorFromContext`); enforcement happens at the route layer. |
| `middleware/request_id.go` | `middleware` | Request-ID middleware: `X-Request-Id` honored if present, else `hostname/base62-counter`; the ID is echoed back in the `X-Request-Id` response header. |
| `middleware/wide_event.go` | `middleware` | Per-request `WideEventContext` (service metadata, timing, status, error), logged via `slog` as JSON. Tail sampling: always keep errors and slow requests, otherwise `SampleFn`/`SampleRate`. Zero-value config gets defaults (rate 0.05, slow threshold 2s); tune via server options `WithSampleRate`/`WithSlowThreshold`/`WithSampleFn`. |
| `lib/jwt/` | `jwt` | HS256 JWT create/verify. `AccessToken`/`RefreshToken` string types, 15 min / 7 day expiries, `TokenGenerator` interface + impl, refresh-token exchange. Claims are flat `map[string]string` plus reserved `exp`/`iat`/`jti`/`typ` (reserved names in caller data are dropped). Every token carries a `typ` claim (`access`/`refresh`) verified with an exact, fail-closed match; the `exp - iat` duration check (±1 s) remains as claim sanity. Refresh tokens carry a random `jti`; `RevocationStore` + `NewTokenGeneratorWithRevocation` enable rotation/denylist (verify fails closed, exchange revokes the old jti). Tests: `jwt_test.go`, `rotation_test.go`, `typ_test.go`. |
| `errors/errors.go` | `errors` | Domain sentinel errors (`ErrBadRequest`, `ErrUnauthenticated`, `ErrUnauthorized`, `ErrNotFound`, `ErrNotImplemented`, `ErrInternalServerError`), re-exported `Is`/`As`/`New`/`Wrap`, and `MapErrorToStatus`/`MapErrorToHumaStatus` (5xx → generic client message, detail server-side only). |
| `config/load.go` | `config` | Generic `Load[T]` via a fresh viper instance per call (no shared state): binds env vars from `mapstructure` tags, optionally reads a config file via `WithConfigFile(path)` (missing explicit file errors; env vars override file values), unmarshals into caller's struct. `AppMode` (`development`/`production`). Logs the loaded config with `sensitive:"true"`-tagged fields redacted (see `redact.go`). |
| `config/redact.go` | `config` | Redaction for the config log: `sensitive:"true"` string fields show only their last 5 chars (`*****ab1de`); ≤5-char and non-string secrets are fully masked. Top-level struct fields only. |
| `utils/` | `utils` | Single helper: `IsStructOrStructPtr` (used by `config.Load`). Prefer stdlib `maps`/`slices` over adding helpers here. |
| `TODO_ITEMS.md` | — | Production-readiness review findings. Read before changing auth, server lifecycle, or error handling. |

## How a request flows

1. `http.ServeMux` → `humago` adapter → huma middleware chain, in this order: **RequestID → Authentication (only if `JWTSigningSecret` set) → WideEvent → caller middlewares** (`server.go:New`).
2. WideEvent skips `SkipPaths` (always includes the OpenAPI/docs/schemas paths plus `WithSkipPaths` extras).
3. `RegisterRoute`'s wrapper handler stamps request ID / method / path / auth_error onto the wide event, converts raw claims to the server-wide `A` via the builder (error → 401; on success `event.UserID = a.UserID()`), then enforces route guards: on a guarded route, no auth info → 401 ("invalid credentials" if bad credentials were presented, else "authentication required"); any guard error → 403; otherwise it calls the typed handler `func(ctx, A, *I) (*O, error)`. Auth failure detail goes to the wide event only, never the response body.
4. Handler errors are mapped via `errors.MapErrorToStatus`/`MapErrorToHumaStatus` (sentinel matching with `errors.Is`); unknown errors become 500. 4xx responses carry the error detail; 5xx responses are generic — the real error goes to the wide event only.

## Conventions

- **Functional options everywhere**: `ServerConfigOption`, `RouterOption`, `RegisterOption` all follow the `With*(...)` + private `loadXxxOptions` pattern. New knobs should follow it too.
- **Domain errors, not status codes, in handlers**: handlers return errors wrapped around the `errors` package sentinels (`fmt.Errorf("...: %w", errors.ErrNotFound)`); the router maps them to HTTP.
- **One AuthInfo type per server.** Auth info is `map[string]string` at the middleware layer (JWT claims, IdP plugin result, context value). The `AuthInfoBuilder[A]` passed to `server.New`/`router.New` converts that map into the server-wide `A` (must implement `AuthInfo`, i.e. `UserID() string`) once per authenticated request, before guards; guards and handlers receive `A`. Wide-event `user_id` attribution comes from `A.UserID()`. Untyped servers use `router.MapAuthInfo`/`MapAuthInfoBuilder`.
- **Context values** use unexported key types with `Get*FromContext` accessors that return zero values on absence — never panic.
- Naming inconsistency to be aware of: `lib/jwt` uses `SCREAMING_SNAKE` consts (`ACCESS_TOKEN_EXPIRY`), non-idiomatic Go; middleware headers likewise (`AUTHORIZATION_HEADER_NAME`). Match local style when editing those files.

## Current state / known issues

`TODO_ITEMS.md` is the authoritative list. Highlights that materially affect any work here:

- Every package has tests and ≥80% statement coverage (root 97%, errors/utils 100%, others 80–88%). Keep it that way: new code ships with tests in the same change.

When fixing any of these, check the corresponding `TODO_ITEMS.md` entry off and update this section.
