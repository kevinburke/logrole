package server

import (
	"encoding/binary"
	"errors"
	"time"

	"github.com/kevinburke/logrole/services"
	"github.com/kevinburke/nacl"
)

// csrfMaxAge is how long a generated CSRF token remains valid.
const csrfMaxAge = 12 * time.Hour

// generateCSRFToken returns an opaque token bound to the secret key and the
// current time. The token has no other binding to the request; this is
// adequate for protecting form actions against off-site POSTs but does not
// provide per-session uniqueness. Panics if secretKey is nil rather than
// silently producing a forgeable token.
func generateCSRFToken(secretKey nacl.Key) string {
	if secretKey == nil {
		panic("server: cannot generate CSRF token with a nil secret key")
	}
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(time.Now().Unix()))
	return services.OpaqueByte(b, secretKey)
}

// verifyCSRFToken returns an error if the token is invalid or has expired.
// Panics if secretKey is nil; a nil key would let every token verify and
// silently disable CSRF protection.
func verifyCSRFToken(token string, secretKey nacl.Key) error {
	if secretKey == nil {
		panic("server: cannot verify CSRF token with a nil secret key")
	}
	b, err := services.UnopaqueByte(token, secretKey)
	if err != nil {
		return errors.New("invalid CSRF token")
	}
	if len(b) != 8 {
		return errors.New("invalid CSRF token payload")
	}
	ts := int64(binary.BigEndian.Uint64(b))
	if time.Since(time.Unix(ts, 0)) > csrfMaxAge {
		return errors.New("CSRF token expired")
	}
	return nil
}
