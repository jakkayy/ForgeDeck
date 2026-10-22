// Package keyring encrypts stored credentials with AES-256-GCM using keys from configuration.
package keyring

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const keyBytes = 32

var validKeyID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

var ErrUnknownKey = errors.New("ciphertext was encrypted with an unknown key")

type Keyring struct {
	activeID string
	keys     map[string]cipher.AEAD
}

// Parse reads "id:base64key[,...]"; the first key seals, the rest only open
func Parse(spec string) (*Keyring, error) {
	ring := &Keyring{keys: map[string]cipher.AEAD{}}
	for i, entry := range strings.Split(spec, ",") {
		id, encoded, ok := strings.Cut(strings.TrimSpace(entry), ":")
		if !ok || !validKeyID.MatchString(id) {
			return nil, fmt.Errorf("key %d: want id:base64key with a lowercase id", i+1)
		}
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(raw) != keyBytes {
			return nil, fmt.Errorf("key %q: must be %d random bytes, base64 encoded", id, keyBytes)
		}
		if _, dup := ring.keys[id]; dup {
			return nil, fmt.Errorf("key %q: listed twice", id)
		}
		block, err := aes.NewCipher(raw)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", id, err)
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", id, err)
		}
		ring.keys[id] = aead
		if i == 0 {
			ring.activeID = id
		}
	}
	return ring, nil
}

// Seal encrypts plaintext with the active key; aad binds it to its owner so it cannot be moved to another row
func (k *Keyring) Seal(plaintext, aad []byte) (ciphertext []byte, keyID string) {
	aead := k.keys[k.activeID]
	nonce := make([]byte, aead.NonceSize())
	_, _ = rand.Read(nonce) // crypto/rand.Read never returns an error.
	return aead.Seal(nonce, nonce, plaintext, aad), k.activeID
}

func (k *Keyring) Open(ciphertext []byte, keyID string, aad []byte) ([]byte, error) {
	aead, ok := k.keys[keyID]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKey, keyID)
	}
	if len(ciphertext) < aead.NonceSize() {
		return nil, errors.New("ciphertext is too short")
	}
	nonce, sealed := ciphertext[:aead.NonceSize()], ciphertext[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, sealed, aad)
	if err != nil {
		return nil, errors.New("ciphertext is corrupt or bound to another record")
	}
	return plaintext, nil
}
