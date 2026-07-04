# TODO — Production Readiness

Review findings from evaluating this library as the HTTP server for `tenzing-agent-harness/cmd/http`. Ordered by severity. File references are current as of the initial commit (`183aa69`).

## Blockers (required before consumers can adopt)

- [x] **Expose the raw mux.** Done via `Server.Handle(pattern, handler)` (`server.go`) — registers raw `http.Handler` routes without exporting the mux. Raw routes bypass the huma middleware chain and the OpenAPI spec; register before `Start`. SSE should go through `huma/v2/sse` instead.
- [x] **Add server timeouts.** Done: defaults read-header 5 s / read 10 s / idle 120 s, overridable via `WithReadHeaderTimeout`/`WithReadTimeout`/`WithIdleTimeout` (`config.go`).
- [x] **`Start` swallows startup errors.** Done: `Start(addr) (<-chan error, error)` — binds synchronously (bind errors returned immediately), serves in a goroutine, serve errors sent on the channel, channel closed on stop (`server.go`). Covered by `server_test.go`.

## Security

- [x] **CRITICAL — Route-guard bypass.** Done: guarded routes now return 401 when `authInfo` is nil and run guards unconditionally otherwise (`router/register.go`). Guard rejections and 401s are also stamped onto the wide event. Covered by `router/register_test.go`.
- [x] **Auth errors are silently dropped.** Done (pass-through-and-record policy): verification failures are stored in context (`AuthErrorContextKey` / `GetAuthErrorFromContext`), stamped onto the wide event (`auth_error` field), and guarded routes return 401 "invalid credentials" (presented-but-bad) vs "authentication required" (absent) — detail never reaches the client. Public routes still serve stale-credential requests as anonymous. Covered by `middleware/authentication_test.go`.
- [x] **Refresh token accepted as request auth.** Done: `BuildAuthInfo` no longer reads the `refresh_token` cookie; only access tokens (`X-App-Key`) and the IdP `Authorization` header authenticate requests. Refresh tokens are honored solely via `ExchangeRefreshToken` at a consumer-built exchange endpoint (`REFRESH_TOKEN_COOKIE_NAME` stays exported for that). Covered by `middleware/authentication_test.go`.
- [x] **No refresh-token rotation/revocation.** Done: refresh tokens carry a random `jti` claim; `RevocationStore` interface (`IsRevoked`/`Revoke`) is the denylist hook (no built-in impl — consumers bring Redis/DB). `NewTokenGeneratorWithRevocation` enforces it: verify fails closed on revoked or jti-less refresh tokens, and every exchange revokes the old jti. Wire via `WithTokenGenerator` (option was previously dead — now honored in `server.New`). Covered by `lib/jwt/rotation_test.go`.
- [x] **No token-type claim.** Done: tokens carry a reserved `typ` claim (`access`/`refresh`); verification requires an exact match and fails closed (missing/wrong `typ` → rejected, caller data cannot forge it). The `exp - iat` duration check remains as a secondary claim-sanity check. Breaking: tokens minted before this change no longer verify. Covered by `lib/jwt/typ_test.go`.
- [x] **Config load prints secrets.** Done: fields tagged `sensitive:"true"` are redacted in the config log — last 5 characters shown (`*****ab1de`) so operators can confirm the right secret loaded; values ≤5 chars and non-string secrets are fully masked (`config/redact.go`). Consumers must tag their secret fields. Covered by `config/redact_test.go`.
- [ ] **Internal errors leak to clients.** `register.go:153-169` passes the wrapped internal error into `huma.Error5xx(...)`, exposing internal detail in response bodies. Return a generic message for 5xx; log the real error server-side.
- [x] **Claims collision.** Done (required by the jti work): `getClaims` skips reserved claim names (`exp`, `iat`, `jti`) when merging caller data, so callers cannot forge expiry or token IDs. Covered by `TestReservedClaimsCannotBeOverridden`.

## Bugs

- [x] **Config log branches inverted.** Done: branches swapped in `config/load.go` (fixed alongside the secret-redaction change in the same function).
- [ ] **Wide-event sampling defaults never applied.** Comments promise `SampleRate` default 0.05 and `SlowThreshold` default 2s (`middleware/wide_event.go:66-67`), but the zero values are used — `Duration > 0` is always true, so every request is logged. Apply the defaults, and expose `SampleRate`/`SlowThreshold`/`SampleFn` through `ServerConfig`/options (currently unreachable from the `server` package API).

## Quality / cleanup

- [ ] `RegisterRoute[I, O any, AuthInfo map[string]string]` (`register.go:60`) constrains `AuthInfo` to exactly `map[string]string`, making the type parameter pointless. Drop it.
- [ ] `register.go:123-170` (`getErrorStatusCode`/`getHumaErrorStatus`) duplicates `errors.MapErrorToStatus`/`MapErrorToHumaStatus`. Delete the copies and use the exported functions.
- [ ] `config.Load` uses the global viper instance — state leaks across loads/tests. Use `viper.New()`.
- [ ] `middleware/request_id.go:52` ignores the `rand.Read` error; the request ID is also never echoed back in a response header.
- [ ] `utils` package reimplements stdlib: `Keys`/`Dedupe` → `maps.Keys` + `slices`, `Map`/`Filter` → `slices` helpers. Delete what stdlib covers.
- [ ] **Test coverage.** Only `lib/jwt` has tests. Middleware, router, and route guards have zero coverage — the guard bypass above would have been caught by one test. Target 80%+.
- [ ] **Repo hygiene.** No README, no LICENSE, no CI, single commit, no version tags. Tag `v0.1.0` once blockers land so consumers can pin a version instead of `@main`.
