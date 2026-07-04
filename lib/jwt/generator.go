package jwt

import (
	"context"
	"time"

	"github.com/tab58/huma-http-server/errors"
)

type TokenGenerator interface {
	GenerateNewTokenPair(ctx context.Context, info map[string]string) (AccessToken, RefreshToken, error)
	ExchangeRefreshToken(ctx context.Context, refreshToken RefreshToken) (AccessToken, RefreshToken, error)
	VerifyAccessToken(ctx context.Context, accessToken AccessToken) (map[string]string, error)
	VerifyRefreshToken(ctx context.Context, refreshToken RefreshToken) (map[string]string, error)
	CreateAccessToken(ctx context.Context, info map[string]string) (AccessToken, error)
	CreateRefreshToken(ctx context.Context, info map[string]string) (RefreshToken, error)
}

type tokenGenerator struct {
	jwtSecret     string
	store         RevocationStore // nil disables revocation checks
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

// GeneratorOption configures a TokenGenerator.
type GeneratorOption func(*tokenGenerator)

// WithAccessTokenExpiry overrides the default access-token lifetime
// (ACCESS_TOKEN_EXPIRY).
func WithAccessTokenExpiry(d time.Duration) GeneratorOption {
	return func(gen *tokenGenerator) {
		gen.accessExpiry = d
	}
}

// WithRefreshTokenExpiry overrides the default refresh-token lifetime
// (REFRESH_TOKEN_EXPIRY).
func WithRefreshTokenExpiry(d time.Duration) GeneratorOption {
	return func(gen *tokenGenerator) {
		gen.refreshExpiry = d
	}
}

func (gen *tokenGenerator) GenerateNewTokenPair(ctx context.Context, info map[string]string) (AccessToken, RefreshToken, error) {
	accessToken, err := gen.CreateAccessToken(ctx, info)
	if err != nil {
		return "", "", err
	}
	refreshToken, err := gen.CreateRefreshToken(ctx, info)
	if err != nil {
		return "", "", err
	}
	return accessToken, refreshToken, nil
}

func (gen *tokenGenerator) VerifyAccessToken(ctx context.Context, accessToken AccessToken) (map[string]string, error) {
	return VerifyAccessToken(ctx, accessToken, gen.jwtSecret)
}

func (gen *tokenGenerator) VerifyRefreshToken(ctx context.Context, refreshToken RefreshToken) (map[string]string, error) {
	info, err := VerifyRefreshToken(ctx, refreshToken, gen.jwtSecret)
	if err != nil {
		return nil, err
	}
	if gen.store != nil {
		// fail closed: with revocation enabled, a refresh token must carry a
		// jti and that jti must not be on the denylist
		jti := info[JTI_CLAIM]
		if jti == "" {
			return nil, errors.Wrap(errors.ErrUnauthenticated, "refresh token missing jti")
		}
		revoked, err := gen.store.IsRevoked(ctx, jti)
		if err != nil {
			return nil, errors.Wrap(errors.ErrInternalServerError, "revocation check failed")
		}
		if revoked {
			return nil, errors.Wrap(errors.ErrUnauthenticated, "refresh token revoked")
		}
	}
	return info, nil
}

func (gen *tokenGenerator) CreateAccessToken(ctx context.Context, info map[string]string) (AccessToken, error) {
	return createAccessToken(info, gen.jwtSecret, gen.accessExpiry)
}

func (gen *tokenGenerator) CreateRefreshToken(ctx context.Context, info map[string]string) (RefreshToken, error) {
	return createRefreshToken(info, gen.jwtSecret, gen.refreshExpiry)
}

// ExchangeRefreshToken verifies the refresh token (including the revocation
// check when a store is configured), issues a new token pair, and revokes the
// old token's jti so it cannot be replayed.
func (gen *tokenGenerator) ExchangeRefreshToken(ctx context.Context, refreshToken RefreshToken) (AccessToken, RefreshToken, error) {
	info, err := gen.VerifyRefreshToken(ctx, refreshToken)
	if err != nil {
		return "", "", errors.Wrap(err, "refresh token exchange failed")
	}

	access, refresh, err := gen.GenerateNewTokenPair(ctx, info)
	if err != nil {
		return "", "", err
	}

	if gen.store != nil {
		// upper bound on the old token's remaining life; the store may drop
		// the entry after this time
		expiresAt := time.Now().Add(gen.refreshExpiry)
		if err := gen.store.Revoke(ctx, info[JTI_CLAIM], expiresAt); err != nil {
			return "", "", errors.Wrap(errors.ErrInternalServerError, "failed to revoke old refresh token")
		}
	}
	return access, refresh, nil
}

func NewTokenGenerator(jwtSecret string, options ...GeneratorOption) TokenGenerator {
	gen := &tokenGenerator{
		jwtSecret:     jwtSecret,
		accessExpiry:  ACCESS_TOKEN_EXPIRY,
		refreshExpiry: REFRESH_TOKEN_EXPIRY,
	}
	for _, option := range options {
		option(gen)
	}
	return gen
}

// NewTokenGeneratorWithRevocation returns a TokenGenerator that enforces
// refresh-token rotation: verification consults the store's denylist and
// every exchange revokes the old token's jti.
func NewTokenGeneratorWithRevocation(jwtSecret string, store RevocationStore, options ...GeneratorOption) TokenGenerator {
	gen := &tokenGenerator{
		jwtSecret:     jwtSecret,
		store:         store,
		accessExpiry:  ACCESS_TOKEN_EXPIRY,
		refreshExpiry: REFRESH_TOKEN_EXPIRY,
	}
	for _, option := range options {
		option(gen)
	}
	return gen
}
