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
- [x] **Internal errors leak to clients.** Done: `errors.MapErrorToHumaStatus` returns generic messages for 5xx ("internal server error" / "not implemented") with no wrapped error attached; 4xx detail still reaches the client (intentional messaging). Real error is logged via the wide event (`SetError`). Covered by `router/register_test.go`.
- [x] **Claims collision.** Done (required by the jti work): `getClaims` skips reserved claim names (`exp`, `iat`, `jti`) when merging caller data, so callers cannot forge expiry or token IDs. Covered by `TestReservedClaimsCannotBeOverridden`.

## Bugs

- [x] **Config log branches inverted.** Done: branches swapped in `config/load.go` (fixed alongside the secret-redaction change in the same function).
- [x] **Wide-event sampling defaults never applied.** Done: `applyWideEventDefaults` applies `DEFAULT_SAMPLE_RATE` (0.05) and `DEFAULT_SLOW_THRESHOLD` (2s) to zero values inside `WideEvent`; `WithSampleRate`/`WithSlowThreshold`/`WithSampleFn` server options wire through to the middleware. Note: an explicit rate of 0 is indistinguishable from unset and becomes 0.05. Covered by `middleware/wide_event_test.go` and `config_test.go`.

## Quality / cleanup

- [x] `RegisterRoute` `AuthInfo` type parameter — resolved the opposite way (owner's call: one typed auth object per server): `AuthInfo` is now an interface (`UserID() string`), the server/router is generic over it (`Server[A]`/`Router[A]`), and a single `AuthInfoBuilder[A]` passed to `New` converts JWT claims into `A` for every route (no per-route builders). `router.MapAuthInfo` + `router.MapAuthInfoBuilder` cover the untyped case. Builder error → 401; wide-event `UserID` comes from `A.UserID()`. Covered by `router/register_test.go`.
- [x] `register.go` duplicate error mappers deleted (done with the 5xx-leak fix); the route wrapper now calls `errors.MapErrorToStatus`/`MapErrorToHumaStatus`.
- [x] `config.Load` global viper — done: each `Load` uses a fresh `viper.New()`; config files are opted into explicitly via `WithConfigFile(path)` (missing explicit file is now a hard error; previously configuring the global viper was the only file mechanism). Covered by `config/load_test.go`.
- [x] `middleware/request_id.go` — done: `rand.Read` failure panics at init (crypto/rand never errors on Go ≥1.24; fail fast if that breaks), and the request ID is echoed in the `X-Request-Id` response header (caller-supplied IDs preserved verbatim). Covered by `middleware/request_id_test.go`.
- [x] `utils` package trimmed: `map.go` and `slices.go` deleted (only `Keys` was used — replaced by `slices.Collect(maps.Keys(...))` at its single call site; `Dedupe`/`Map`/`MapErr`/`Run`/`Filter` were dead), `StructToMap` deleted (unused). Package is now just `IsStructOrStructPtr` (used by `config.Load`).
- [x] **Test coverage.** Done — every package ≥80%: root 97.4%, config 87.7%, errors 100%, lib/jwt 84.2%, middleware 87.9%, router 80.7%, utils 100% (`go test ./... -cover`, all passing with `-race`).
- [ ] **Repo hygiene.** README done (features, compile-checked quick start, auth/config/observability guides). Still missing: LICENSE, CI, version tags. Tag `v0.1.0` so consumers can pin a version instead of `@main` — all blockers/security/bug items above are resolved.
