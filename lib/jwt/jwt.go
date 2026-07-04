package jwt

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	lib "github.com/golang-jwt/jwt/v5"
	"github.com/tab58/huma-http-server/errors"
)

type (
	MapClaims = lib.MapClaims
)

// reservedClaims are managed by this package and can never be set (or
// overridden) via caller-supplied data.
var reservedClaims = map[string]struct{}{
	"exp":     {},
	"iat":     {},
	JTI_CLAIM: {},
	TYP_CLAIM: {},
}

func getClaims(data map[string]string, timeout time.Duration) lib.MapClaims {
	// create base claims
	claims := lib.MapClaims{
		"exp": time.Now().Add(timeout).Unix(),
		"iat": time.Now().Unix(),
	}

	// merge metadata claims into claims, skipping reserved names
	for key, value := range data {
		if _, reserved := reservedClaims[key]; reserved {
			continue
		}
		claims[key] = value
	}

	return claims
}

// claimUnix reads a Unix-seconds numeric claim. golang-jwt parses JSON numbers
// as float64; claims built in-process carry int64.
func claimUnix(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	}
	return 0, false
}

func verifyClaims(claims lib.MapClaims, timeout time.Duration, expectedTyp string) (map[string]string, error) {
	// Verify the token type claim (fail closed: absent typ is rejected)
	if typ, _ := claims[TYP_CLAIM].(string); typ != expectedTyp {
		return nil, fmt.Errorf("unexpected token type: %w", errors.ErrUnauthenticated)
	}

	// Verify expiry and iat claims
	expiry, ok := claimUnix(claims["exp"])
	if !ok {
		return nil, fmt.Errorf("missing or invalid exp claim: %w", errors.ErrBadRequest)
	}
	issuedAt, ok := claimUnix(claims["iat"])
	if !ok {
		return nil, fmt.Errorf("missing or invalid iat claim: %w", errors.ErrBadRequest)
	}
	tOut := time.Unix(expiry, 0).Sub(time.Unix(issuedAt, 0))

	if math.Abs(float64(timeout-tOut)) > float64(1*time.Second) {
		return nil, fmt.Errorf("invalid timeout: %w", errors.ErrBadRequest)
	}

	data := make(map[string]string)
	for key, value := range claims {
		if key == "exp" || key == "iat" {
			continue
		}
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("non-string claim %q: %w", key, errors.ErrBadRequest)
		}
		data[key] = s
	}

	return data, nil
}

type signingData struct {
	Method lib.SigningMethod
	Secret []byte
}

func verifyJWT(token string, timeout time.Duration, signer *signingData, expectedTyp string) (map[string]string, error) {
	// parse and verify the JWT token
	parsedToken, err := lib.Parse(token, func(token *lib.Token) (any, error) {
		// verify the signing method
		if _, ok := token.Method.(*lib.SigningMethodHMAC); !ok {
			return nil, errors.Wrap(errors.ErrUnauthenticated, "unexpected signing method")
		}
		return []byte(signer.Secret), nil
	})
	if err != nil {
		return nil, errors.Wrap(errors.ErrUnauthenticated, "failed to parse JWT")
	}

	// extract claims
	claims, ok := parsedToken.Claims.(lib.MapClaims)
	if !ok {
		return nil, errors.Wrap(errors.ErrUnauthenticated, "failed to extract claims from JWT")
	}
	return verifyClaims(claims, timeout, expectedTyp)
}

// createJWT signs a token with the given data claims. The typ claim declares
// the token type; a non-empty jti is set as the "jti" claim. Both are
// reserved — caller data cannot supply them.
func createJWT(data map[string]string, timeout time.Duration, signer *signingData, jti string, typ string) (string, error) {
	claims := getClaims(data, timeout)
	claims[TYP_CLAIM] = typ
	if jti != "" {
		claims[JTI_CLAIM] = jti
	}
	token := lib.NewWithClaims(signer.Method, claims)
	tokenString, err := token.SignedString(signer.Secret)
	if err != nil {
		return "", errors.Wrap(errors.ErrInternalServerError, "failed to sign JWT")
	}
	return tokenString, nil
}
