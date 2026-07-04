package jwt

import (
	"context"
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
	jwtSecret string
}

func (gen *tokenGenerator) GenerateNewTokenPair(ctx context.Context, info map[string]string) (AccessToken, RefreshToken, error) {
	return GenerateNewTokenPair(ctx, info, gen.jwtSecret)
}

func (gen *tokenGenerator) VerifyAccessToken(ctx context.Context, accessToken AccessToken) (map[string]string, error) {
	return VerifyAccessToken(ctx, accessToken, gen.jwtSecret)
}

func (gen *tokenGenerator) VerifyRefreshToken(ctx context.Context, refreshToken RefreshToken) (map[string]string, error) {
	return VerifyRefreshToken(ctx, refreshToken, gen.jwtSecret)
}

func (gen *tokenGenerator) CreateAccessToken(ctx context.Context, info map[string]string) (AccessToken, error) {
	return CreateAccessToken(ctx, info, gen.jwtSecret)
}

func (gen *tokenGenerator) CreateRefreshToken(ctx context.Context, info map[string]string) (RefreshToken, error) {
	return CreateRefreshToken(ctx, info, gen.jwtSecret)
}

func (gen *tokenGenerator) ExchangeRefreshToken(ctx context.Context, refreshToken RefreshToken) (AccessToken, RefreshToken, error) {
	return ExchangeRefreshToken(ctx, refreshToken, gen.jwtSecret)
}

func NewTokenGenerator(jwtSecret string) TokenGenerator {
	return &tokenGenerator{
		jwtSecret: jwtSecret,
	}
}
