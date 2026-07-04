package router

import "context"

// USER_ID_CLAIM is the claim key MapAuthInfo reads for UserID. Mint tokens
// with this key to get per-user log attribution out of the box.
const USER_ID_CLAIM = "user_id"

// AuthInfo is the contract every server-wide auth type must satisfy. UserID
// is used for wide-event log attribution and must be derivable from the JWT
// claims your AuthInfoBuilder receives.
type AuthInfo interface {
	UserID() string
}

// AuthInfoBuilder converts the raw claims from the auth middleware into the
// server's AuthInfo type. It runs once per authenticated request, before
// route guards; a returned error yields 401.
type AuthInfoBuilder[A AuthInfo] func(ctx context.Context, raw map[string]string) (A, error)

// MapAuthInfo is the pass-through AuthInfo for servers that don't need a
// typed auth object: the raw claims map itself, with UserID read from the
// "user_id" claim.
type MapAuthInfo map[string]string

func (m MapAuthInfo) UserID() string {
	return m[USER_ID_CLAIM]
}

// MapAuthInfoBuilder is the builder for MapAuthInfo servers.
func MapAuthInfoBuilder(ctx context.Context, raw map[string]string) (MapAuthInfo, error) {
	return MapAuthInfo(raw), nil
}
