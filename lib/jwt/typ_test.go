package jwt

import (
	"context"
	"testing"
	"time"

	lib "github.com/golang-jwt/jwt/v5"
)

// signRaw builds a token with arbitrary claims, bypassing createJWT — used to
// simulate forged or legacy tokens.
func signRaw(t *testing.T, claims lib.MapClaims) string {
	t.Helper()
	token, err := lib.NewWithClaims(JWT_SIGNING_METHOD, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("SignedString: %v", err)
	}
	return token
}

func TestTokensCarryTypClaim(t *testing.T) {
	ctx := context.Background()

	access, err := CreateAccessToken(ctx, map[string]string{"sub": "u1"}, testSecret)
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	info, err := VerifyAccessToken(ctx, access, testSecret)
	if err != nil {
		t.Fatalf("VerifyAccessToken: %v", err)
	}
	if info[TYP_CLAIM] != TOKEN_TYPE_ACCESS {
		t.Errorf("access token typ = %q, want %q", info[TYP_CLAIM], TOKEN_TYPE_ACCESS)
	}

	refresh, err := CreateRefreshToken(ctx, map[string]string{"sub": "u1"}, testSecret)
	if err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	rinfo, err := VerifyRefreshToken(ctx, refresh, testSecret)
	if err != nil {
		t.Fatalf("VerifyRefreshToken: %v", err)
	}
	if rinfo[TYP_CLAIM] != TOKEN_TYPE_REFRESH {
		t.Errorf("refresh token typ = %q, want %q", rinfo[TYP_CLAIM], TOKEN_TYPE_REFRESH)
	}
}

func TestWrongTypRejectedEvenWithMatchingExpiry(t *testing.T) {
	// a token with access-token expiry but typ=refresh must fail access
	// verification — proves typ is checked, not just the exp-iat duration
	ctx := context.Background()
	now := time.Now()
	token := signRaw(t, lib.MapClaims{
		"exp":     now.Add(ACCESS_TOKEN_EXPIRY).Unix(),
		"iat":     now.Unix(),
		TYP_CLAIM: TOKEN_TYPE_REFRESH,
		"sub":     "u1",
	})
	if _, err := VerifyAccessToken(ctx, AccessToken(token), testSecret); err == nil {
		t.Fatal("access verification accepted a token with typ=refresh")
	}
}

func TestTokenWithoutTypRejected(t *testing.T) {
	// legacy tokens predating the typ claim must fail closed
	ctx := context.Background()
	now := time.Now()
	token := signRaw(t, lib.MapClaims{
		"exp": now.Add(ACCESS_TOKEN_EXPIRY).Unix(),
		"iat": now.Unix(),
		"sub": "u1",
	})
	if _, err := VerifyAccessToken(ctx, AccessToken(token), testSecret); err == nil {
		t.Fatal("access verification accepted a token with no typ claim")
	}
}

func TestCallerCannotForgeTypViaInfo(t *testing.T) {
	ctx := context.Background()
	token, err := CreateAccessToken(ctx, map[string]string{"sub": "u1", TYP_CLAIM: TOKEN_TYPE_REFRESH}, testSecret)
	if err != nil {
		t.Fatalf("CreateAccessToken: %v", err)
	}
	// typ from caller data must be dropped; the token still verifies as access
	if _, err := VerifyAccessToken(ctx, token, testSecret); err != nil {
		t.Fatalf("VerifyAccessToken: %v — caller-supplied typ should be ignored", err)
	}
}
