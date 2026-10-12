// Package keyring encrypts stored credentials with AES-256-GCM. Keys come
// from configuration, never the database, and carry an ID so ciphertext
// written with an older key can still be opened after rotation.
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

// ErrUnknownKey means ciphertext was sealed with a key that is not configured.
var ErrUnknownKey = errors.New("ciphertext was encrypted with an unknown key")

// Keyring holds the active key and any older keys still needed for reading.
type Keyring struct {
	activeID string
	keys     map[string]cipher.AEAD
}

// Parse reads "id:base64key[,id:base64key...]". The first key is used for new
// ciphertext; the rest only decrypt. Generate a key with: openssl rand -base64 32
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

// Seal encrypts plaintext with the active key. aad binds the ciphertext to
// its owner (for example "git_connection:<id>:token"), so it cannot be
// copied into another row and decrypted there.
func (k *Keyring) Seal(plaintext, aad []byte) (ciphertext []byte, keyID string) {
	aead := k.keys[k.activeID]
	nonce := make([]byte, aead.NonceSize())
	_, _ = rand.Read(nonce) // crypto/rand.Read never returns an error.
	return aead.Seal(nonce, nonce, plaintext, aad), k.activeID
}

// Open decrypts ciphertext produced by Seal with keyID and the same aad.
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
