package webui

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookieName = "molex_session"
	maxLoginFailures  = 5
	loginWindow       = time.Minute
)

type sessionContextKey struct{}

type authenticatedSession struct {
	key     [32]byte
	csrf    string
	expires time.Time
}

type sessionStore struct {
	mu       sync.Mutex
	sessions map[[32]byte]authenticatedSession
	now      func() time.Time
}

func newSessionStore(now func() time.Time) *sessionStore {
	return &sessionStore{sessions: make(map[[32]byte]authenticatedSession), now: now}
}

func (s *sessionStore) create(ttl time.Duration) (string, authenticatedSession, error) {
	token, err := randomToken()
	if err != nil {
		return "", authenticatedSession{}, err
	}
	csrf, err := randomToken()
	if err != nil {
		return "", authenticatedSession{}, err
	}
	session := authenticatedSession{key: hashToken(token), csrf: csrf, expires: s.now().Add(ttl)}
	s.mu.Lock()
	s.removeExpiredLocked()
	s.sessions[session.key] = session
	s.mu.Unlock()
	return token, session, nil
}

func (s *sessionStore) get(token string) (authenticatedSession, bool) {
	key := hashToken(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked()
	session, ok := s.sessions[key]
	return session, ok
}

func (s *sessionStore) valid(key [32]byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeExpiredLocked()
	_, ok := s.sessions[key]
	return ok
}

func (s *sessionStore) delete(key [32]byte) {
	s.mu.Lock()
	delete(s.sessions, key)
	s.mu.Unlock()
}

func (s *sessionStore) removeExpiredLocked() {
	now := s.now()
	for key, session := range s.sessions {
		if !now.Before(session.expires) {
			delete(s.sessions, key)
		}
	}
}

type loginAttempt struct {
	failures int
	resetAt  time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
	now      func() time.Time
}

func newLoginLimiter(now func() time.Time) *loginLimiter {
	return &loginLimiter{attempts: make(map[string]loginAttempt), now: now}
}

func (l *loginLimiter) allowed(client string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	attempt, ok := l.attempts[client]
	if !ok || !now.Before(attempt.resetAt) {
		delete(l.attempts, client)
		return true, 0
	}
	if attempt.failures < maxLoginFailures {
		return true, 0
	}
	return false, attempt.resetAt.Sub(now)
}

func (l *loginLimiter) failure(client string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	attempt, ok := l.attempts[client]
	if !ok || !now.Before(attempt.resetAt) {
		attempt = loginAttempt{resetAt: now.Add(loginWindow)}
	}
	attempt.failures++
	l.attempts[client] = attempt
}

func (l *loginLimiter) success(client string) {
	l.mu.Lock()
	delete(l.attempts, client)
	l.mu.Unlock()
}

func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := s.sessionFromRequest(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), sessionContextKey{}, session)))
	}
}

func (s *Server) sessionFromRequest(r *http.Request) (authenticatedSession, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		return authenticatedSession{}, false
	}
	return s.sessions.get(cookie.Value)
}

func sessionFromContext(ctx context.Context) (authenticatedSession, bool) {
	session, ok := ctx.Value(sessionContextKey{}).(authenticatedSession)
	return session, ok
}

func requireCSRF(w http.ResponseWriter, r *http.Request) bool {
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "cross-origin request rejected")
		return false
	}
	session, ok := sessionFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return false
	}
	provided := r.Header.Get("X-MoleX-CSRF")
	if subtle.ConstantTimeCompare([]byte(provided), []byte(session.csrf)) != 1 {
		writeError(w, http.StatusForbidden, "invalid CSRF token")
		return false
	}
	return true
}

func (s *Server) passwordMatches(password string) bool {
	candidate := hashToken(password)
	return subtle.ConstantTimeCompare(candidate[:], s.passwordHash[:]) == 1
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   max(1, int(ttl/time.Second)),
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   requestIsSecure(r),
		SameSite: http.SameSiteStrictMode,
	})
}

func sameOrigin(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.User == nil && strings.EqualFold(parsed.Host, r.Host)
}

func requestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback() && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func randomToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func hashToken(value string) [32]byte {
	return sha256.Sum256([]byte(value))
}

func validatePasswordInput(password string) error {
	if len(password) > 1024 {
		return errors.New("password is too long")
	}
	return nil
}
