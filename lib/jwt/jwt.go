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

func getClaims(data map[string]string, timeout time.Duration) lib.MapClaims {
	// create base claims
	claims := lib.MapClaims{
		"exp": time.Now().Add(timeout).Unix(),
		"iat": time.Now().Unix(),
	}

	// merge metadata claims into claims
	for key, value := range data {
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

func verifyClaims(claims lib.MapClaims, timeout time.Duration) (map[string]string, error) {
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

func verifyJWT(token string, timeout time.Duration, signer *signingData) (map[string]string, error) {
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
	return verifyClaims(claims, timeout)
}

func createJWT(data map[string]string, timeout time.Duration, signer *signingData) (string, error) {
	claims := getClaims(data, timeout)
	token := lib.NewWithClaims(signer.Method, claims)
	tokenString, err := token.SignedString(signer.Secret)
	if err != nil {
		return "", errors.Wrap(errors.ErrInternalServerError, "failed to sign JWT")
	}
	return tokenString, nil
}
