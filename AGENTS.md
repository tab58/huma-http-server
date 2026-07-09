# AGENTS.md

Root documentation for this repository. Source of truth for layout, behavior, and conventions (see `CLAUDE.md` for the doc-maintenance rules). Update this file in the same change whenever code alters anything described here.

## What this is

A reusable Go library (module `github.com/tab58/huma-http-server`, Go 1.25) that wraps [Huma v2](https://github.com/danielgtaylor/huma) into an opinionated HTTP API server with batteries included:

- OpenAPI 3.1 docs/schemas served automatically (`/openapi`, `/docs`, `/schemas`)
- JWT access/refresh token auth (HS256) with an optional external IdP plugin
- Request-ID injection
- "Wide event" structured request logging with tail sampling
- Typed route registration with per-route guard functions and domain-error → HTTP-status mapping
- Optional CORS handling (`WithCORS`) and in-process TLS (`StartTLS`)

It is a library, not an application: there is no `main`, no config file, no Dockerfile. Consumers import `server` (root package), call `server.New(...)`, register routes via `router.RegisterRoute`, then `Start`/`Shutdown`.

## Build / test

No Taskfile/Makefile/CI. Plain Go toolchain:

```bash
go build ./...
go vet ./...
go test ./...
go test ./lib/jwt -run TestAccessTokenRoundTrip   # single test
go test ./... -cover -race
```

## Layout

| Path | Package | Purpose |
|------|---------|---------|
| `server.go` | `server` | Entry point. `Server[A router.AuthInfo]`; `New[A](cfg, builder, options...)` wires middleware + router with the server-wide auth type, `Start(addr) (<-chan error, error)` binds synchronously then serves in a goroutine (serve errors on the channel; closed on stop); `StartTLS(addr, certFile, keyFile)` same but terminates TLS in-process; `Shutdown` for graceful stop. The mux is wrapped at the HTTP layer by `middleware.WideEventNotFound` (404/405 wide events) and, when `WithCORS` is set, `middleware.CORS` outermost. `Handle(pattern, handler)` mounts raw `http.Handler` routes (bypass huma middlewares and OpenAPI; register before `Start`). HTTP server timeouts set from options (defaults: read-header 5 s, read 10 s, idle 120 s; no write timeout). |
| `config.go` | `server` | `ServerConfig`, `serverConfigOptions`, and all `With*` functional options (OpenAPI paths, formats, token generator, IdP plugin, middlewares, skip paths, timeouts, wide-event sampling/logger, CORS). `WithSampleRate` defaults to 0.05; passing 0 disables success sampling. |
| `register.go` | `server` | Convenience re-export: `RegisterRoute(s *Server[A], args, opts...)` delegates to `router.RegisterRoute` so consumers only import the root package. |
| `router/router.go` | `router` | `Router[A AuthInfo]`: `http.ServeMux` + `humago` adapter + the server-wide `AuthInfoBuilder[A]`; middlewares attach via `api.UseMiddleware`. |
| `router/auth_info.go` | `router` | `AuthInfo` interface (`UserID() string`) — the contract every server-wide auth type implements. `AuthInfoBuilder[A]` (claims → `A`), `MapAuthInfo` (raw-map pass-through, `UserID` from the `user_id` claim / `USER_ID_CLAIM`), `MapAuthInfoBuilder`. |
| `router/register.go` | `router` | `RegisterRoute[I, O any, A AuthInfo](r *Router[A], args, opts...)` — typed route registration. Pulls request ID / auth info / wide event from context, converts raw claims to `A` via the router's builder (error → 401), stamps `event.UserID` from `A.UserID()`, runs route guards, maps handler errors to `huma.StatusError`. |
| `middleware/authentication.go` | `middleware` | Auth middleware. Checks `X-App-Key` header (access token), then `Authorization` header via `IdPPlugin`. Refresh tokens never authenticate requests — they are only for the token-exchange endpoint (`ExchangeRefreshToken`). Puts `map[string]string` auth info in context. Verification failure continues unauthenticated but records the error in context (`GetAuthErrorFromContext`); enforcement happens at the route layer. |
| `middleware/request_id.go` | `middleware` | Request-ID middleware: `X-Request-Id` honored if present, else `hostname/base62-counter`; the ID is echoed back in the `X-Request-Id` response header. |
| `middleware/wide_event.go` | `middleware` | Per-request `WideEventContext` (service metadata, request ID, method/path, timing, status, error), logged via `slog.LogAttrs` as a structured `event` attr (`Logger` field, default `slog.Default()`). The middleware stamps request ID/method/path itself and backfills the status from `ctx.Status()`, so huma 422 validation failures carry full identity. `WideEventNotFound(cfg, mux)` wraps the mux to log requests matching no route (404/405). Tail sampling: always keep errors (status ≥ 400) and slow requests, otherwise `SampleFn`/`SampleRate`. `SampleRate` 0 means no success sampling (no zero-default); slow threshold zero-defaults to 2s. |
| `middleware/cors.go` | `middleware` | `CORS(cfg, next)` http-level middleware + `CORSConfig`. Explicit `AllowedOrigins` required (`"*"` supported); answers preflight OPTIONS itself (204); disallowed origins pass through with no CORS headers. Wired via server option `WithCORS`. |
| `lib/jwt/` | `jwt` | HS256 JWT create/verify. `AccessToken`/`RefreshToken` string types, default 15 min / 7 day expiries (per-generator override via `WithAccessTokenExpiry`/`WithRefreshTokenExpiry`), `TokenGenerator` interface + impl, refresh-token exchange (generator method only — it enforces revocation). Claims are flat `map[string]string` plus reserved `exp`/`iat`/`jti`/`typ` (reserved names in caller data are dropped). Verification: parser requires `exp` (30 s leeway, `CLOCK_SKEW_LEEWAY`), `typ` claim (`access`/`refresh`) matched exactly and fail-closed; non-string claims (`nbf`, array `aud`, numeric customs) are tolerated and dropped from the claims map; all verification failures map to `ErrUnauthenticated` (401). Refresh tokens carry a random `jti`; `RevocationStore` + `NewTokenGeneratorWithRevocation` enable rotation/denylist (verify fails closed, exchange revokes the old jti). Known limits: HS256 only, no key rotation, no `iss`/`aud` validation. Tests: `jwt_test.go`, `rotation_test.go`, `typ_test.go`. |
| `errors/errors.go` | `errors` | Domain sentinel errors (`ErrBadRequest`, `ErrUnauthenticated`, `ErrUnauthorized`, `ErrNotFound`, `ErrConflict`, `ErrTooManyRequests`, `ErrNotImplemented`, `ErrInternalServerError`), re-exported `Is`/`As`/`New`/`Wrap`, and `MapErrorToStatus`/`MapErrorToHumaStatus` (5xx → generic client message, detail server-side only). Errors already implementing `huma.StatusError` pass through unchanged — handlers can return e.g. `huma.Error409Conflict` directly. |
| `config/load.go` | `config` | Generic `Load[T]` via a fresh viper instance per call (no shared state): binds env vars from `mapstructure` tags, applies `default:"..."` tag values as fallbacks (precedence: env > file > default), optionally reads a config file via `WithConfigFile(path)` (missing explicit file errors), unmarshals into caller's struct. `AppMode` (`development`/`production`). Silent by default; `WithConfigDump()` logs the loaded config via `slog` with `sensitive:"true"`-tagged fields redacted (see `redact.go`). |
| `config/redact.go` | `config` | Redaction for the config log: `sensitive:"true"` string fields show only their last 5 chars (`*****ab1de`); ≤5-char and non-string secrets are fully masked. Recurses into nested structs (types with custom JSON marshaling, e.g. `time.Time`, pass through as-is). |
| `utils/` | `utils` | Single helper: `IsStructOrStructPtr` (used by `config.Load`). Prefer stdlib `maps`/`slices` over adding helpers here. |
| `TODO_ITEMS.md` | — | Production-readiness review findings. Read before changing auth, server lifecycle, or error handling. |

## How a request flows

1. HTTP layer first: **CORS (if `WithCORS`) → WideEventNotFound** — preflight OPTIONS is answered before route matching, and requests matching no mux route get a 404/405 wide event. Then `http.ServeMux` → `humago` adapter → huma middleware chain, in this order: **RequestID → Authentication (only if `JWTSigningSecret` set) → WideEvent → caller middlewares** (`server.go:New`).
2. WideEvent skips `SkipPaths` (always includes the OpenAPI/docs/schemas paths plus `WithSkipPaths` extras). It stamps request ID/method/path up front and backfills the response status after the chain runs, so huma validation failures (422) log with full request identity.
3. `RegisterRoute`'s wrapper handler stamps request ID / method / path / auth_error onto the wide event, converts raw claims to the server-wide `A` via the builder (error → 401; on success `event.UserID = a.UserID()`), then enforces route guards: on a guarded route, no auth info → 401 ("invalid credentials" if bad credentials were presented, else "authentication required"); any guard error → 403; otherwise it calls the typed handler `func(ctx, A, *I) (*O, error)`. Auth failure detail goes to the wide event only, never the response body.
4. Handler errors are mapped via `errors.MapErrorToStatus`/`MapErrorToHumaStatus` (sentinel matching with `errors.Is`); errors already implementing `huma.StatusError` pass through unchanged; unknown errors become 500. 4xx responses carry the error detail; 5xx responses are generic — the real error goes to the wide event only.

## Conventions

- **Functional options everywhere**: `ServerConfigOption`, `RouterOption`, `RegisterOption` all follow the `With*(...)` + private `loadXxxOptions` pattern. New knobs should follow it too.
- **Domain errors, not status codes, in handlers**: handlers return errors wrapped around the `errors` package sentinels (`fmt.Errorf("...: %w", errors.ErrNotFound)`); the router maps them to HTTP.
- **One AuthInfo type per server.** Auth info is `map[string]string` at the middleware layer (JWT claims, IdP plugin result, context value). The `AuthInfoBuilder[A]` passed to `server.New`/`router.New` converts that map into the server-wide `A` (must implement `AuthInfo`, i.e. `UserID() string`) once per authenticated request, before guards; guards and handlers receive `A`. Wide-event `user_id` attribution comes from `A.UserID()`. Untyped servers use `router.MapAuthInfo`/`MapAuthInfoBuilder`.
- **Context values** use unexported key types with `Get*FromContext` accessors that return zero values on absence — never panic.
- Naming inconsistency to be aware of: `lib/jwt` uses `SCREAMING_SNAKE` consts (`ACCESS_TOKEN_EXPIRY`), non-idiomatic Go; middleware headers likewise (`AUTHORIZATION_HEADER_NAME`). Match local style when editing those files.

## Current state / known issues

`TODO_ITEMS.md` is the authoritative list; its 2026-07-03 review findings are all resolved (see the checked-off entries there). Remaining known limits, deliberate and documented rather than bugs: HS256 only (no signing-key rotation, no `iss`/`aud` validation), and rate limiting is left to a reverse proxy or caller middleware.

- Every package has tests and ≥80% statement coverage (root 95%, errors/utils 100%, others 80–94%). Keep it that way: new code ships with tests in the same change.

When fixing or adding known issues, keep `TODO_ITEMS.md` and this section in sync.
