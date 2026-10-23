package app

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/ratelimit"
)

// The limits are per process, in memory (ForgeDeck runs as one Control Plane process)
const (
	userMutationBurst    = 60
	userMutationInterval = time.Second

	failureBurst    = 20
	failureInterval = 6 * time.Second

	enrollBurst    = 10
	enrollInterval = 6 * time.Second
)

// strictUserLimits tighten per-user limits on guessable or outbound-calling routes
var strictUserLimits = map[string]struct {
	burst    int
	interval time.Duration
}{
	"POST " + "/api/v1/projects/{projectID}/members":                   {10, 6 * time.Second},
	"POST " + "/api/v1/projects/{projectID}/servers/enrollment-tokens": {10, 6 * time.Second},
	"POST " + "/api/v1/projects/{projectID}/git-connections/gitlab":    {10, 6 * time.Second},
	"POST " + "/api/v1/projects/{projectID}/git-connections/github":    {10, 6 * time.Second},
	"POST " + "/api/v1/projects":                                       {10, 6 * time.Second},
	"POST " + "/api/v1/runners":                                        {10, 6 * time.Second},
	"POST " + "/api/v1/applications/{applicationID}/builds":            {30, 2 * time.Second},
	"PUT " + "/api/v1/projects/{projectID}/secrets/{name}":             {30, 2 * time.Second},
}

// limiters holds the rate limiters of one router
type limiters struct {
	mutations *ratelimit.Limiter
	strict    map[string]*ratelimit.Limiter
	failures  *ratelimit.Limiter
	enroll    *ratelimit.Limiter
}

func newLimiters(now func() time.Time) *limiters {
	l := &limiters{
		mutations: ratelimit.New(userMutationInterval, userMutationBurst, now),
		strict:    map[string]*ratelimit.Limiter{},
		failures:  ratelimit.New(failureInterval, failureBurst, now),
		enroll:    ratelimit.New(enrollInterval, enrollBurst, now),
	}
	for route, limit := range strictUserLimits {
		l.strict[route] = ratelimit.New(limit.interval, limit.burst, now)
	}
	return l
}

// userMutations limits how fast a signed-in user may change things
func (l *limiters) userMutations(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		user, ok := auth.MaybeUserFrom(r.Context())
		if !ok {
			next.ServeHTTP(w, r)
			return
		}
		key := user.ID.String()
		if strict, ok := l.strict[r.Method+" "+chi.RouteContext(r.Context()).RoutePattern()]; ok {
			if allowed, retry := strict.Allow(key); !allowed {
				httpx.WriteError(w, r, httpx.TooManyRequests(w, retry))
				return
			}
		}
		if allowed, retry := l.mutations.Allow(key); !allowed {
			httpx.WriteError(w, r, httpx.TooManyRequests(w, retry))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// enrollment limits how fast one address may try enrollment tokens
func (l *limiters) enrollment(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && chi.RouteContext(r.Context()).RoutePattern() == "/api/v1/agent/enroll" {
			if allowed, retry := l.enroll.Allow(httpx.ClientIP(r)); !allowed {
				httpx.WriteError(w, r, httpx.TooManyRequests(w, retry))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
