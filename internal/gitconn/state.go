package gitconn

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/keyring"
)

const installStateTTL = 15 * time.Minute

var (
	installStateAAD   = []byte("github-install-state")
	errInvalidState   = errors.New("install state is invalid")
	errExpiredInstall = errors.New("install state has expired")
)

// installState is sealed with the keyring and binds the callback to the user and project
type installState struct {
	ProjectID uuid.UUID `json:"p"`
	UserID    uuid.UUID `json:"u"`
	ExpiresAt int64     `json:"e"`
}

func sealState(ring *keyring.Keyring, state installState) string {
	payload, _ := json.Marshal(state) // fixed struct, cannot fail
	ciphertext, keyID := ring.Seal(payload, installStateAAD)
	return keyID + "." + base64.RawURLEncoding.EncodeToString(ciphertext)
}

func openState(ring *keyring.Keyring, raw string, now time.Time) (installState, error) {
	keyID, encoded, ok := strings.Cut(raw, ".")
	if !ok {
		return installState{}, errInvalidState
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return installState{}, errInvalidState
	}
	payload, err := ring.Open(ciphertext, keyID, installStateAAD)
	if err != nil {
		return installState{}, errInvalidState
	}
	var state installState
	if err := json.Unmarshal(payload, &state); err != nil {
		return installState{}, errInvalidState
	}
	if now.Unix() > state.ExpiresAt {
		return installState{}, errExpiredInstall
	}
	return state, nil
}
