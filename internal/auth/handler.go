package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/ratelimit"
)

// Login attempts are limited per client IP and per email. The per-email limit
// still protects an account when every request arrives from one proxy IP.
const (
	loginBurstPerIP     = 20
	loginRefillPerIP    = 3 * time.Second
	loginBurstPerEmail  = 10
	loginRefillPerEmail = 30 * time.Second
)

// Handler serves the /auth endpoints.
type Handler struct {
	svc          *Service
	cookie       cookieConfig
	loginByIP    *ratelimit.Limiter
	loginByEmail *ratelimit.Limiter
}

// NewHandler returns auth endpoints. secureCookies must be true when the
// Control Plane is served over HTTPS.
func NewHandler(svc *Service, secureCookies bool) *Handler {
	return &Handler{
		svc:          svc,
		cookie:       newCookieConfig(secureCookies),
		loginByIP:    ratelimit.New(loginRefillPerIP, loginBurstPerIP, time.Now),
		loginByEmail: ratelimit.New(loginRefillPerEmail, loginBurstPerEmail, time.Now),
	}
}

// Routes mounts the auth endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/auth/login", httpx.Handle(h.login))
	r.Post("/auth/logout", httpx.Handle(h.logout))
	r.With(h.RequireUser).Get("/auth/me", httpx.Handle(h.me))
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) error {
	var req loginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := h.limitLogin(w, r, req.Email); err != nil {
		return err
	}

	user, session, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		return httpx.Unauthorized("Email or password is incorrect.")
	}
	if err != nil {
		return err
	}

	h.cookie.set(w, session.Token)
	return httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"user": user,
	})
}

func (h *Handler) limitLogin(w http.ResponseWriter, r *http.Request, email string) error {
	if ok, retry := h.loginByIP.Allow(httpx.ClientIP(r)); !ok {
		return httpx.TooManyRequests(w, retry)
	}
	if ok, retry := h.loginByEmail.Allow(strings.ToLower(strings.TrimSpace(email))); !ok {
		return httpx.TooManyRequests(w, retry)
	}
	return nil
}

// logout ends the current session, if any, and clears the cookie. It succeeds
// even without a valid session so the client can always reach a signed-out state.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) error {
	if token, ok := h.cookie.read(r); ok {
		if err := h.svc.EndSession(r.Context(), token); err != nil {
			return err
		}
	}
	h.cookie.clear(w)
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) error {
	return httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": UserFrom(r.Context())})
}

// cookieConfig names and scopes the session cookie. Over HTTPS the __Host-
// prefix pins the cookie to this exact origin.
type cookieConfig struct {
	name   string
	secure bool
}

func newCookieConfig(secure bool) cookieConfig {
	if secure {
		return cookieConfig{name: "__Host-forgedeck_session", secure: true}
	}
	return cookieConfig{name: "forgedeck_session"}
}

func (c cookieConfig) set(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     c.name,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionMaxLifetime / time.Second),
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteStrictMode,
	})
}

func (c cookieConfig) read(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(c.name)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func (c cookieConfig) clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     c.name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   c.secure,
		SameSite: http.SameSiteStrictMode,
	})
}
