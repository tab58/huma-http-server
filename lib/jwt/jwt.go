package jwt

import (
	"fmt"
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

func verifyClaims(claims lib.MapClaims, expectedTyp string) (map[string]string, error) {
	// Verify the token type claim (fail closed: absent typ is rejected)
	if typ, _ := claims[TYP_CLAIM].(string); typ != expectedTyp {
		return nil, fmt.Errorf("unexpected token type: %w", errors.ErrUnauthenticated)
	}

	// exp validity is enforced by the parser (WithExpirationRequired +
	// leeway). Keep only string claims: registered numeric/array claims
	// (exp, iat, nbf, aud, ...) and non-string custom claims are dropped,
	// not rejected — externally minted tokens may carry them.
	data := make(map[string]string)
	for key, value := range claims {
		if s, ok := value.(string); ok {
			data[key] = s
		}
	}

	return data, nil
}

type signingData struct {
	Method lib.SigningMethod
	Secret []byte
}

func verifyJWT(token string, signer *signingData, expectedTyp string) (map[string]string, error) {
	// parse and verify the JWT token; exp is required and validated by the
	// parser (with leeway for clock skew), so verification failures — expired,
	// forged, malformed — all map to 401, never 400
	parsedToken, err := lib.Parse(token, func(token *lib.Token) (any, error) {
		// verify the signing method
		if _, ok := token.Method.(*lib.SigningMethodHMAC); !ok {
			return nil, errors.Wrap(errors.ErrUnauthenticated, "unexpected signing method")
		}
		return []byte(signer.Secret), nil
	}, lib.WithExpirationRequired(), lib.WithLeeway(CLOCK_SKEW_LEEWAY))
	if err != nil {
		return nil, errors.Wrap(errors.ErrUnauthenticated, "failed to parse JWT")
	}

	// extract claims
	claims, ok := parsedToken.Claims.(lib.MapClaims)
	if !ok {
		return nil, errors.Wrap(errors.ErrUnauthenticated, "failed to extract claims from JWT")
	}
	return verifyClaims(claims, expectedTyp)
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
