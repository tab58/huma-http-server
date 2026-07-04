package middleware

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/tab58/huma-http-server/lib/jwt"
)

const AUTHORIZATION_HEADER_NAME = "Authorization"
const ACCESS_TOKEN_HEADER_NAME = "X-App-Key"
const REFRESH_TOKEN_COOKIE_NAME = "refresh_token"

type ctxKeyAuthInfo struct{}

var AuthContextKey ctxKeyAuthInfo = ctxKeyAuthInfo{}

type IdPPlugin interface {
	ValidateAuthorizationHeader(ctx context.Context, authHeader string) (map[string]string, error)
}

type Authenticator struct {
	Generator        jwt.TokenGenerator
	IdentityProvider IdPPlugin
}

func (a *Authenticator) GenerateNewTokenPair(ctx context.Context, info map[string]string) (jwt.AccessToken, jwt.RefreshToken, error) {
	return a.Generator.GenerateNewTokenPair(ctx, info)
}

func (a *Authenticator) ExchangeRefreshToken(ctx context.Context, refreshToken jwt.RefreshToken) (jwt.AccessToken, jwt.RefreshToken, error) {
	return a.Generator.ExchangeRefreshToken(ctx, refreshToken)
}

func (a *Authenticator) BuildAuthInfo(c huma.Context) (map[string]string, error) {
	ctx := c.Context()
	authHeader := c.Header(AUTHORIZATION_HEADER_NAME)
	accessToken := c.Header(ACCESS_TOKEN_HEADER_NAME)
	var refreshToken string = ""
	if refreshTokenCookie, err := huma.ReadCookie(c, REFRESH_TOKEN_COOKIE_NAME); err == nil {
		refreshToken = refreshTokenCookie.Value
	}

	var authInfo map[string]string

	// check the tokens for auth info
	if accessToken != "" {
		tokenInfo, err := a.Generator.VerifyAccessToken(ctx, jwt.AccessToken(accessToken))
		if err != nil {
			return nil, err
		}
		authInfo = tokenInfo
	} else if refreshToken != "" {
		tokenInfo, err := a.Generator.VerifyRefreshToken(ctx, jwt.RefreshToken(refreshToken))
		if err != nil {
			return nil, err
		}
		authInfo = tokenInfo
	} else if authHeader != "" && a.IdentityProvider != nil {
		tokenInfo, err := a.IdentityProvider.ValidateAuthorizationHeader(ctx, authHeader)
		if err != nil {
			return nil, err
		}
		authInfo = tokenInfo
	}

	return authInfo, nil
}

func Authentication(authenticator Authenticator) func(ctx huma.Context, next func(huma.Context)) {
	return func(c huma.Context, next func(huma.Context)) {
		if authInfo, err := authenticator.BuildAuthInfo(c); err == nil {
			next(huma.WithValue(c, AuthContextKey, authInfo))
		} else {
			next(c)
		}
	}
}

func GetAuthInfoFromContext(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}
	if authInfo, ok := ctx.Value(AuthContextKey).(map[string]string); ok {
		return authInfo
	}
	return nil
}
