// Package auth holds the credential primitives of the account feature:
// argon2id password hashing in PHC string form and opaque session tokens.
//
// Password hashes embed their parameters ($argon2id$v=19$m=...,t=...,p=...
// $salt$hash), so verification reads the parameters back out of the stored
// string and a future parameter change needs no migration. Session tokens
// are random and only their SHA-256 is ever stored — a read of the
// sessions table cannot steal a live session.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters for new hashes, per current OWASP guidance.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 2
	argonSaltLen = 16
	argonKeyLen  = 32
)

// HashPassword returns a PHC-formatted argon2id hash of password.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	sum := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum)), nil
}

// VerifyPassword reports whether password matches the PHC hash produced by
// HashPassword. It returns false for malformed input rather than erroring —
// a bad stored hash must not read as "correct".
func VerifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	// PHC: "$argon2id$v=19$m=...,t=...,p=...$salt$hash" → 6 parts, first empty.
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return false
	}
	version, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
	if err != nil || version != argon2.Version {
		return false
	}
	var memory, time, threads int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(time), uint32(memory), uint8(threads), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

// NewToken returns a new opaque session token: 32 bytes of crypto/rand
// encoded as unpadded base64url (~43 chars).
func NewToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// HashToken returns the hex SHA-256 of a session token — the only form the
// database ever sees.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
