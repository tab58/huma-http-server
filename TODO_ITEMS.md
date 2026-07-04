# TODO — Production Readiness

Review findings from evaluating this library as the HTTP server for `tenzing-agent-harness/cmd/http`. Ordered by severity. File references are current as of the initial commit (`183aa69`).

## Blockers (required before consumers can adopt)

- [x] **Expose the raw mux.** Done via `Server.Handle(pattern, handler)` (`server.go`) — registers raw `http.Handler` routes without exporting the mux. Raw routes bypass the huma middleware chain and the OpenAPI spec; register before `Start`. SSE should go through `huma/v2/sse` instead.
- [x] **Add server timeouts.** Done: defaults read-header 5 s / read 10 s / idle 120 s, overridable via `WithReadHeaderTimeout`/`WithReadTimeout`/`WithIdleTimeout` (`config.go`).
- [x] **`Start` swallows startup errors.** Done: `Start(addr) (<-chan error, error)` — binds synchronously (bind errors returned immediately), serves in a goroutine, serve errors sent on the channel, channel closed on stop (`server.go`). Covered by `server_test.go`.

## Security

- [x] **CRITICAL — Route-guard bypass.** Done: guarded routes now return 401 when `authInfo` is nil and run guards unconditionally otherwise (`router/register.go`). Guard rejections and 401s are also stamped onto the wide event. Covered by `router/register_test.go`.
- [x] **Auth errors are silently dropped.** Done (pass-through-and-record policy): verification failures are stored in context (`AuthErrorContextKey` / `GetAuthErrorFromContext`), stamped onto the wide event (`auth_error` field), and guarded routes return 401 "invalid credentials" (presented-but-bad) vs "authentication required" (absent) — detail never reaches the client. Public routes still serve stale-credential requests as anonymous. Covered by `middleware/authentication_test.go`.
- [ ] **Refresh token accepted as request auth.** `authentication.go:53-58`: a refresh-token cookie authenticates any route. Refresh tokens should only be honored at the token-exchange endpoint. Cookie-based auth also has no CSRF protection.
- [ ] **No refresh-token rotation/revocation.** `ExchangeRefreshToken` issues a new pair but the old refresh token stays valid for its full 7-day life. Add a `jti` claim and a revocation/denylist hook so stolen tokens can be invalidated.
- [ ] **No token-type claim.** Access vs. refresh tokens are distinguished only by `exp - iat` duration ±1s (`lib/jwt/jwt.go:57-61`). Add an explicit `typ` claim and verify it.
- [ ] **Config load prints secrets.** `config/load.go:59` dumps the full config JSON (including `JWTSigningSecret`) to stdout. Remove or redact sensitive fields.
- [ ] **Internal errors leak to clients.** `register.go:153-169` passes the wrapped internal error into `huma.Error5xx(...)`, exposing internal detail in response bodies. Return a generic message for 5xx; log the real error server-side.
- [ ] **Claims collision.** `lib/jwt/jwt.go:17-30` merges caller-supplied `info` over the base claims, so an `"exp"` or `"iat"` key in `info` overwrites the real expiry. Reject or skip reserved claim names.

## Bugs

- [ ] **Config log branches inverted.** `config/load.go:41-45`: on `ReadInConfig` error it prints "Using config file: ..."; on success it prints "config file not found". Swap the branches.
- [ ] **Wide-event sampling defaults never applied.** Comments promise `SampleRate` default 0.05 and `SlowThreshold` default 2s (`middleware/wide_event.go:66-67`), but the zero values are used — `Duration > 0` is always true, so every request is logged. Apply the defaults, and expose `SampleRate`/`SlowThreshold`/`SampleFn` through `ServerConfig`/options (currently unreachable from the `server` package API).

## Quality / cleanup

- [ ] `RegisterRoute[I, O any, AuthInfo map[string]string]` (`register.go:60`) constrains `AuthInfo` to exactly `map[string]string`, making the type parameter pointless. Drop it.
- [ ] `register.go:123-170` (`getErrorStatusCode`/`getHumaErrorStatus`) duplicates `errors.MapErrorToStatus`/`MapErrorToHumaStatus`. Delete the copies and use the exported functions.
- [ ] `config.Load` uses the global viper instance — state leaks across loads/tests. Use `viper.New()`.
- [ ] `middleware/request_id.go:52` ignores the `rand.Read` error; the request ID is also never echoed back in a response header.
- [ ] `utils` package reimplements stdlib: `Keys`/`Dedupe` → `maps.Keys` + `slices`, `Map`/`Filter` → `slices` helpers. Delete what stdlib covers.
- [ ] **Test coverage.** Only `lib/jwt` has tests. Middleware, router, and route guards have zero coverage — the guard bypass above would have been caught by one test. Target 80%+.
- [ ] **Repo hygiene.** No README, no LICENSE, no CI, single commit, no version tags. Tag `v0.1.0` once blockers land so consumers can pin a version instead of `@main`.
