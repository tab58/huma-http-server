package jwt

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	lib "github.com/golang-jwt/jwt/v5"
	"github.com/tab58/huma-http-server/errors"
)

var JWT_SIGNING_METHOD = lib.SigningMethodHS256

const ACCESS_TOKEN_EXPIRY = 15 * time.Minute
const REFRESH_TOKEN_EXPIRY = 7 * 24 * time.Hour

// JTI_CLAIM is the claim holding the unique token ID on refresh tokens,
// used for rotation/revocation. Reserved — cannot be set via caller data.
const JTI_CLAIM = "jti"

// TYP_CLAIM is the claim declaring the token type. Reserved — cannot be set
// via caller data. Verification requires an exact match, so access and
// refresh tokens can never be substituted for each other.
const TYP_CLAIM = "typ"

const TOKEN_TYPE_ACCESS = "access"
const TOKEN_TYPE_REFRESH = "refresh"

// RevocationStore is the denylist hook for refresh-token revocation. The
// library ships no implementation: production deployments need a shared
// store (Redis, DB) so revocation holds across instances. Entries can be
// dropped after expiresAt.
type RevocationStore interface {
	IsRevoked(ctx context.Context, jti string) (bool, error)
	Revoke(ctx context.Context, jti string, expiresAt time.Time) error
}

func newJTI() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", errors.Wrap(errors.ErrInternalServerError, "failed to generate jti")
	}
	return hex.EncodeToString(buf[:]), nil
}

func ExchangeRefreshToken(ctx context.Context, refreshToken RefreshToken, jwtSecret string) (AccessToken, RefreshToken, error) {
	tokenInfo, err := VerifyRefreshToken(ctx, refreshToken, jwtSecret)
	if err != nil {
		return "", "", fmt.Errorf("%w: %w", errors.ErrUnauthenticated, err)
	}

	newAccessToken, newRefreshToken, err := GenerateNewTokenPair(ctx, tokenInfo, jwtSecret)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate new tokens: %w: %w", errors.ErrInternalServerError, err)
	}

	return newAccessToken, newRefreshToken, nil
}

// GenerateNewTokenPair generates a new access and refresh token pair
func GenerateNewTokenPair(ctx context.Context, info map[string]string, jwtSecret string) (AccessToken, RefreshToken, error) {
	accessToken, err := CreateAccessToken(ctx, info, jwtSecret)
	if err != nil {
		return "", "", fmt.Errorf("failed to create access token: %w: %w", errors.ErrInternalServerError, err)
	}
	refreshToken, err := CreateRefreshToken(ctx, info, jwtSecret)
	if err != nil {
		return "", "", fmt.Errorf("failed to create refresh token: %w: %w", errors.ErrInternalServerError, err)
	}
	return accessToken, refreshToken, nil
}

// VerifyAccessToken verifies an access token and returns the token info if valid
func VerifyAccessToken(ctx context.Context, accessToken AccessToken, jwtSecret string) (map[string]string, error) {
	signingData := &signingData{
		Method: JWT_SIGNING_METHOD,
		Secret: []byte(jwtSecret),
	}
	return verifyJWT(string(accessToken), ACCESS_TOKEN_EXPIRY, signingData, TOKEN_TYPE_ACCESS)
}

// VerifyRefreshToken verifies a refresh token and returns the token info if valid
func VerifyRefreshToken(ctx context.Context, refreshToken RefreshToken, jwtSecret string) (map[string]string, error) {
	signingData := &signingData{
		Method: JWT_SIGNING_METHOD,
		Secret: []byte(jwtSecret),
	}
	return verifyJWT(string(refreshToken), REFRESH_TOKEN_EXPIRY, signingData, TOKEN_TYPE_REFRESH)
}

// CreateAccessToken creates a new access token
func CreateAccessToken(ctx context.Context, info map[string]string, jwtSecret string) (AccessToken, error) {
	token, err := createJWT(info, ACCESS_TOKEN_EXPIRY, &signingData{
		Method: JWT_SIGNING_METHOD,
		Secret: []byte(jwtSecret),
	}, "", TOKEN_TYPE_ACCESS)

	if err != nil {
		return "", fmt.Errorf("failed to create access token: %w: %w", errors.ErrInternalServerError, err)
	}
	return AccessToken(token), nil
}

// CreateRefreshToken creates a new refresh token carrying a fresh jti claim.
func CreateRefreshToken(ctx context.Context, info map[string]string, jwtSecret string) (RefreshToken, error) {
	jti, err := newJTI()
	if err != nil {
		return "", err
	}
	token, err := createJWT(info, REFRESH_TOKEN_EXPIRY, &signingData{
		Method: JWT_SIGNING_METHOD,
		Secret: []byte(jwtSecret),
	}, jti, TOKEN_TYPE_REFRESH)
	if err != nil {
		return "", fmt.Errorf("failed to create refresh token: %w: %w", errors.ErrInternalServerError, err)
	}
	return RefreshToken(token), nil
}
