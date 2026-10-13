package keyring

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
)

func randomKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, keyBytes)
	_, _ = rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func TestSealOpenRoundTrip(t *testing.T) {
	ring, err := Parse("v1:" + randomKey(t))
	if err != nil {
		t.Fatal(err)
	}
	aad := []byte("git_connection:1:token")
	ciphertext, keyID := ring.Seal([]byte("glpat-secret"), aad)
	if keyID != "v1" || bytes.Contains(ciphertext, []byte("glpat-secret")) {
		t.Fatalf("keyID %q ciphertext %q", keyID, ciphertext)
	}
	plaintext, err := ring.Open(ciphertext, keyID, aad)
	if err != nil || string(plaintext) != "glpat-secret" {
		t.Fatalf("Open = %q, %v", plaintext, err)
	}

	again, _ := ring.Seal([]byte("glpat-secret"), aad)
	if bytes.Equal(again, ciphertext) {
		t.Fatal("nonce reused: identical ciphertext")
	}
}

func TestOpenRejectsTamperingAndWrongOwner(t *testing.T) {
	ring, _ := Parse("v1:" + randomKey(t))
	ciphertext, keyID := ring.Seal([]byte("secret"), []byte("row:1"))

	if _, err := ring.Open(ciphertext, keyID, []byte("row:2")); err == nil {
		t.Error("ciphertext opened for another record")
	}
	tampered := bytes.Clone(ciphertext)
	tampered[len(tampered)-1] ^= 1
	if _, err := ring.Open(tampered, keyID, []byte("row:1")); err == nil {
		t.Error("tampered ciphertext opened")
	}
	if _, err := ring.Open(ciphertext, "v9", []byte("row:1")); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("unknown key: got %v", err)
	}
}

func TestRotationKeepsOldCiphertextReadable(t *testing.T) {
	oldKey, newKey := randomKey(t), randomKey(t)
	before, _ := Parse("v1:" + oldKey)
	ciphertext, keyID := before.Seal([]byte("secret"), nil)

	after, err := Parse("v2:" + newKey + ", v1:" + oldKey)
	if err != nil {
		t.Fatal(err)
	}
	if plaintext, err := after.Open(ciphertext, keyID, nil); err != nil || string(plaintext) != "secret" {
		t.Fatalf("old ciphertext after rotation: %q %v", plaintext, err)
	}
	if _, activeID := after.Seal([]byte("x"), nil); activeID != "v2" {
		t.Fatalf("active key %q, want v2", activeID)
	}
}

func TestParseRejectsBadSpecs(t *testing.T) {
	for _, spec := range []string{
		"",
		"v1",
		"V1:" + randomKey(t),
		"v1:not-base64!",
		"v1:" + base64.StdEncoding.EncodeToString([]byte("short")),
		"v1:" + randomKey(t) + ",v1:" + randomKey(t),
	} {
		if _, err := Parse(spec); err == nil {
			t.Errorf("accepted %q", spec)
		}
	}
}
