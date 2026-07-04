package jwt

import (
	"context"
	"testing"
	"time"

	lib "github.com/golang-jwt/jwt/v5"
	"github.com/tab58/huma-http-server/errors"
)

const testSecret = "test-signing-secret"

func TestAccessTokenRoundTrip(t *testing.T) {
	ctx := context.Background()
	token, err := CreateAccessToken(ctx, map[string]string{"sub": "user-1", "role": "admin"}, testSecret)
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}

	// golang-jwt parses JSON numbers as float64 — verification must survive
	// the round trip (exp/iat are written as int64 but read back as float64).
	info, err := VerifyAccessToken(ctx, token, testSecret)
	if err != nil {
		t.Fatalf("VerifyAccessToken on a freshly minted token: %v", err)
	}
	if info["sub"] != "user-1" || info["role"] != "admin" {
		t.Errorf("claims = %v, want sub=user-1 role=admin", info)
	}
}

func TestRefreshTokenRoundTrip(t *testing.T) {
	ctx := context.Background()
	token, err := CreateRefreshToken(ctx, map[string]string{"sub": "user-1"}, testSecret)
	if err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	info, err := VerifyRefreshToken(ctx, token, testSecret)
	if err != nil {
		t.Fatalf("VerifyRefreshToken on a freshly minted token: %v", err)
	}
	if info["sub"] != "user-1" {
		t.Errorf("claims = %v, want sub=user-1", info)
	}
}

func TestGeneratorAccessTokenRoundTrip(t *testing.T) {
	ctx := context.Background()
	gen := NewTokenGenerator(testSecret)

	token, err := gen.CreateAccessToken(ctx, map[string]string{"sub": "user-1"})
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	info, err := gen.VerifyAccessToken(ctx, token)
	if err != nil {
		t.Fatalf("VerifyAccessToken: %v", err)
	}
	if info["sub"] != "user-1" {
		t.Errorf("claims = %v, want sub=user-1", info)
	}
}

func TestVerifyAccessTokenRejectsBadInput(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		token string
	}{
		{"garbage", "not-a-jwt"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := VerifyAccessToken(ctx, AccessToken(tt.token), testSecret); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}

	t.Run("wrong secret", func(t *testing.T) {
		token, err := CreateAccessToken(ctx, map[string]string{"sub": "user-1"}, testSecret)
		if err != nil {
			t.Fatalf("CreateAccessToken: %v", err)
		}
		if _, err := VerifyAccessToken(ctx, token, "other-secret"); err == nil {
			t.Error("expected error for wrong secret, got nil")
		}
	})

	t.Run("refresh token rejected as access token", func(t *testing.T) {
		// The typ claim check must reject refresh tokens used as access tokens.
		token, err := CreateRefreshToken(ctx, map[string]string{"sub": "user-1"}, testSecret)
		if err != nil {
			t.Fatalf("CreateRefreshToken: %v", err)
		}
		if _, err := VerifyAccessToken(ctx, AccessToken(token), testSecret); err == nil {
			t.Error("expected error for refresh token used as access token, got nil")
		}
	})
}

func TestVerificationFailuresMapTo401(t *testing.T) {
	ctx := context.Background()
	now := time.Now()

	tests := []struct {
		name   string
		claims lib.MapClaims
	}{
		{"expired token", lib.MapClaims{
			"exp":     now.Add(-2 * time.Minute).Unix(),
			"iat":     now.Add(-17 * time.Minute).Unix(),
			TYP_CLAIM: TOKEN_TYPE_ACCESS,
			"sub":     "u1",
		}},
		{"missing exp rejected", lib.MapClaims{
			"iat":     now.Unix(),
			TYP_CLAIM: TOKEN_TYPE_ACCESS,
			"sub":     "u1",
		}},
		{"wrong typ", lib.MapClaims{
			"exp":     now.Add(ACCESS_TOKEN_EXPIRY).Unix(),
			"iat":     now.Unix(),
			TYP_CLAIM: TOKEN_TYPE_REFRESH,
			"sub":     "u1",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := signRaw(t, tt.claims)
			_, err := VerifyAccessToken(ctx, AccessToken(token), testSecret)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, errors.ErrUnauthenticated) {
				t.Errorf("verification failure should map to ErrUnauthenticated (401), got %v", err)
			}
		})
	}
}

func TestCustomExpiryTokensVerify(t *testing.T) {
	// Tokens minted with non-default expiries must still verify — changing
	// the configured expiry cannot invalidate outstanding tokens.
	ctx := context.Background()
	gen := NewTokenGenerator(testSecret, WithAccessTokenExpiry(time.Hour), WithRefreshTokenExpiry(30*24*time.Hour))

	access, err := gen.CreateAccessToken(ctx, map[string]string{"sub": "u1"})
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	// verifies with a default-expiry generator too
	if _, err := NewTokenGenerator(testSecret).VerifyAccessToken(ctx, access); err != nil {
		t.Fatalf("token with custom expiry rejected: %v", err)
	}

	refresh, err := gen.CreateRefreshToken(ctx, map[string]string{"sub": "u1"})
	if err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	if _, err := NewTokenGenerator(testSecret).VerifyRefreshToken(ctx, refresh); err != nil {
		t.Fatalf("refresh token with custom expiry rejected: %v", err)
	}
}

func TestNonStringClaimsTolerated(t *testing.T) {
	// Externally minted tokens may carry nbf, array aud, or numeric custom
	// claims — verification keeps string claims and drops the rest.
	ctx := context.Background()
	now := time.Now()
	token := signRaw(t, lib.MapClaims{
		"exp":     now.Add(ACCESS_TOKEN_EXPIRY).Unix(),
		"iat":     now.Unix(),
		"nbf":     now.Unix(),
		"aud":     []string{"svc-a", "svc-b"},
		"count":   42,
		TYP_CLAIM: TOKEN_TYPE_ACCESS,
		"sub":     "u1",
	})
	info, err := VerifyAccessToken(ctx, AccessToken(token), testSecret)
	if err != nil {
		t.Fatalf("VerifyAccessToken rejected non-string claims: %v", err)
	}
	if info["sub"] != "u1" {
		t.Errorf("string claim lost: info = %v", info)
	}
	for _, dropped := range []string{"nbf", "aud", "count", "exp", "iat"} {
		if _, present := info[dropped]; present {
			t.Errorf("non-string claim %q should be dropped from info", dropped)
		}
	}
}
