package jwt

import (
	"context"
	"fmt"
	"time"

	lib "github.com/golang-jwt/jwt/v5"
	"github.com/tab58/huma-http-server/errors"
)

var JWT_SIGNING_METHOD = lib.SigningMethodHS256

const ACCESS_TOKEN_EXPIRY = 15 * time.Minute
const REFRESH_TOKEN_EXPIRY = 7 * 24 * time.Hour

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
	return verifyJWT(string(accessToken), ACCESS_TOKEN_EXPIRY, signingData)
}

// VerifyRefreshToken verifies a refresh token and returns the token info if valid
func VerifyRefreshToken(ctx context.Context, refreshToken RefreshToken, jwtSecret string) (map[string]string, error) {
	signingData := &signingData{
		Method: JWT_SIGNING_METHOD,
		Secret: []byte(jwtSecret),
	}
	return verifyJWT(string(refreshToken), REFRESH_TOKEN_EXPIRY, signingData)
}

// CreateAccessToken creates a new access token
func CreateAccessToken(ctx context.Context, info map[string]string, jwtSecret string) (AccessToken, error) {
	token, err := createJWT(info, ACCESS_TOKEN_EXPIRY, &signingData{
		Method: JWT_SIGNING_METHOD,
		Secret: []byte(jwtSecret),
	})

	if err != nil {
		return "", fmt.Errorf("failed to create access token: %w: %w", errors.ErrInternalServerError, err)
	}
	return AccessToken(token), nil
}

// CreateRefreshToken creates a new refresh token
func CreateRefreshToken(ctx context.Context, info map[string]string, jwtSecret string) (RefreshToken, error) {
	token, err := createJWT(info, REFRESH_TOKEN_EXPIRY, &signingData{
		Method: JWT_SIGNING_METHOD,
		Secret: []byte(jwtSecret),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create refresh token: %w: %w", errors.ErrInternalServerError, err)
	}
	return RefreshToken(token), nil
}
