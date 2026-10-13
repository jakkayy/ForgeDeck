package secrets

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/redact"
)

// redactorTTL bounds how long a masking function is reused. Setting or
// deleting a secret through this process refreshes it at once; the TTL only
// matters when another process changes secrets.
const redactorTTL = 15 * time.Second

// Redactor masks a project's secret values in text. It keeps one decrypted
// masking function per project for a short time, because log batches arrive
// often and decrypting every secret for each would be wasteful.
type Redactor struct {
	svc *Service
	now func() time.Time

	mu    sync.Mutex
	cache map[uuid.UUID]cached
}

type cached struct {
	redact  func(string) string
	expires time.Time
}

// NewRedactor returns a Redactor that follows svc's changes.
func NewRedactor(svc *Service) *Redactor {
	r := &Redactor{svc: svc, now: time.Now, cache: map[uuid.UUID]cached{}}
	svc.onChange = r.invalidate
	return r
}

// For returns a function that masks the project's secrets in a string.
func (r *Redactor) For(ctx context.Context, projectID uuid.UUID) (func(string) string, error) {
	r.mu.Lock()
	entry, ok := r.cache[projectID]
	r.mu.Unlock()
	if ok && r.now().Before(entry.expires) {
		return entry.redact, nil
	}

	values, err := r.svc.AllValues(ctx, projectID)
	if err != nil {
		return nil, err
	}
	redactFn := redact.New(values)
	r.mu.Lock()
	r.cache[projectID] = cached{redact: redactFn, expires: r.now().Add(redactorTTL)}
	r.mu.Unlock()
	return redactFn, nil
}

func (r *Redactor) invalidate(projectID uuid.UUID) {
	r.mu.Lock()
	delete(r.cache, projectID)
	r.mu.Unlock()
}
