package jwt

import (
	"context"
	"testing"
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
		// Refresh tokens carry a 7d expiry — the access-token timeout check must reject them.
		token, err := CreateRefreshToken(ctx, map[string]string{"sub": "user-1"}, testSecret)
		if err != nil {
			t.Fatalf("CreateRefreshToken: %v", err)
		}
		if _, err := VerifyAccessToken(ctx, AccessToken(token), testSecret); err == nil {
			t.Error("expected error for refresh token used as access token, got nil")
		}
	})
}
