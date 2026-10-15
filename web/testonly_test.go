package web

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/base64"
	"math/big"
	"net/http"
	"strings"
)

// Helpers only the tests use.

// routes builds the application's handler without competition storage.
func routes() http.Handler {
	return routesWith(nil)
}

// verifyVapidToken checks a token's signature, for tests.
func verifyVapidToken(pub *ecdsa.PublicKey, token string) bool {
	i := strings.LastIndex(token, ".")
	if i < 0 {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(token[i+1:])
	if err != nil || len(sig) != 64 {
		return false
	}
	digest := sha256.Sum256([]byte(token[:i]))
	return ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:]))
}
