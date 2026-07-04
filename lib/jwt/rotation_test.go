package jwt

import (
	"context"
	"sync"
	"testing"
	"time"
)

// memStore is a test-only in-memory RevocationStore. Production consumers
// need a shared store (Redis/DB) — this exists only to exercise the hook.
type memStore struct {
	mu      sync.Mutex
	revoked map[string]bool
}

func newMemStore() *memStore {
	return &memStore{revoked: make(map[string]bool)}
}

func (s *memStore) IsRevoked(ctx context.Context, jti string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revoked[jti], nil
}

func (s *memStore) Revoke(ctx context.Context, jti string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked[jti] = true
	return nil
}

func TestRefreshTokenCarriesJTI(t *testing.T) {
	ctx := context.Background()
	token, err := CreateRefreshToken(ctx, map[string]string{"sub": "user-1"}, testSecret)
	if err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	info, err := VerifyRefreshToken(ctx, token, testSecret)
	if err != nil {
		t.Fatalf("VerifyRefreshToken: %v", err)
	}
	if info[JTI_CLAIM] == "" {
		t.Fatal("refresh token has no jti claim")
	}
}

func TestReservedClaimsCannotBeOverridden(t *testing.T) {
	ctx := context.Background()
	// a malicious caller trying to extend expiry or forge a jti via info
	info := map[string]string{"sub": "user-1", "exp": "99999999999", "iat": "0", "jti": "forged"}
	token, err := CreateRefreshToken(ctx, info, testSecret)
	if err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	got, err := VerifyRefreshToken(ctx, token, testSecret)
	if err != nil {
		t.Fatalf("VerifyRefreshToken: %v — reserved claims from info must be dropped, not break expiry", err)
	}
	if got[JTI_CLAIM] == "forged" {
		t.Fatal("caller-supplied jti overrode the generated one")
	}
}

func TestExchangeRevokesOldRefreshToken(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gen := NewTokenGeneratorWithRevocation(testSecret, store)

	_, refresh, err := gen.GenerateNewTokenPair(ctx, map[string]string{"sub": "user-1"})
	if err != nil {
		t.Fatalf("GenerateNewTokenPair: %v", err)
	}

	// first exchange succeeds
	_, newRefresh, err := gen.ExchangeRefreshToken(ctx, refresh)
	if err != nil {
		t.Fatalf("first ExchangeRefreshToken: %v", err)
	}

	// replaying the old refresh token must fail
	if _, _, err := gen.ExchangeRefreshToken(ctx, refresh); err == nil {
		t.Fatal("old refresh token accepted after rotation")
	}

	// the new refresh token still works
	if _, err := gen.VerifyRefreshToken(ctx, newRefresh); err != nil {
		t.Fatalf("rotated refresh token rejected: %v", err)
	}
}

func TestRevokedRefreshTokenRejected(t *testing.T) {
	ctx := context.Background()
	store := newMemStore()
	gen := NewTokenGeneratorWithRevocation(testSecret, store)

	refresh, err := gen.CreateRefreshToken(ctx, map[string]string{"sub": "user-1"})
	if err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	info, err := gen.VerifyRefreshToken(ctx, refresh)
	if err != nil {
		t.Fatalf("VerifyRefreshToken before revocation: %v", err)
	}
	if err := store.Revoke(ctx, info[JTI_CLAIM], time.Now().Add(REFRESH_TOKEN_EXPIRY)); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := gen.VerifyRefreshToken(ctx, refresh); err == nil {
		t.Fatal("revoked refresh token accepted")
	}
}

func TestRefreshTokenWithoutJTIRejectedWhenStoreConfigured(t *testing.T) {
	ctx := context.Background()
	gen := NewTokenGeneratorWithRevocation(testSecret, newMemStore())

	// craft a legacy refresh token with no jti
	legacy, err := createJWT(map[string]string{"sub": "user-1"}, REFRESH_TOKEN_EXPIRY, &signingData{
		Method: JWT_SIGNING_METHOD,
		Secret: []byte(testSecret),
	}, "", TOKEN_TYPE_REFRESH)
	if err != nil {
		t.Fatalf("createJWT: %v", err)
	}
	if _, err := gen.VerifyRefreshToken(ctx, RefreshToken(legacy)); err == nil {
		t.Fatal("refresh token without jti accepted while revocation store configured")
	}
}

func TestVerifyWithoutStoreIgnoresRevocation(t *testing.T) {
	ctx := context.Background()
	gen := NewTokenGenerator(testSecret)

	refresh, err := gen.CreateRefreshToken(ctx, map[string]string{"sub": "user-1"})
	if err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}
	if _, err := gen.VerifyRefreshToken(ctx, refresh); err != nil {
		t.Fatalf("VerifyRefreshToken without store: %v", err)
	}
}
