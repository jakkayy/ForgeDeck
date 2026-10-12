// Package securetoken generates high-entropy bearer tokens and the hashes
// stored in place of them (sessions, enrollment tokens, Agent credentials).
package securetoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

const tokenBytes = 32

// New returns a random URL-safe token and its SHA-256 hash. Store only the hash.
func New(prefix string) (token string, hash []byte) {
	b := make([]byte, tokenBytes)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error.
	token = prefix + base64.RawURLEncoding.EncodeToString(b)
	return token, Hash(token)
}

// Hash returns the SHA-256 hash used to look a token up. Tokens carry 256 bits
// of entropy, so a fast hash is sufficient; passwords use argon2id instead.
func Hash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
