package relay

import (
	"crypto/sha256"
	"strings"
	"sync"
	"time"

	"github.com/suifei/molex/internal/protocol"
)

// Credential is one admission token the relay accepts. The relay stores the
// value to authenticate clients and to pin each hello to the expected opaque
// route; it never uses it to decrypt tunnel payloads. During a rotation
// grace window the previous value stays valid until PreviousExpires.
type Credential struct {
	ID              string
	Token           string
	Disabled        bool
	ExpiresAt       time.Time
	Previous        string
	PreviousExpires time.Time
}

type tokenState struct {
	id       string
	token    string
	disabled bool
	legacy   bool
	expires  time.Time
	route    [32]byte
}

type tokenStore struct {
	mu     sync.RWMutex
	byHash map[[32]byte]tokenState
	now    func() time.Time
}

func newTokenStore(credentials []Credential) *tokenStore {
	store := &tokenStore{now: time.Now}
	store.replace(credentials)
	return store
}

func (s *tokenStore) replace(credentials []Credential) {
	byHash := make(map[[32]byte]tokenState, len(credentials)*2)
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	for _, credential := range credentials {
		value := strings.TrimSpace(credential.Token)
		if value == "" || credential.ID == "" {
			continue
		}
		byHash[sha256.Sum256([]byte(value))] = tokenState{
			id:       credential.ID,
			token:    value,
			disabled: credential.Disabled,
			expires:  credential.ExpiresAt,
			route:    protocol.RouteForToken(value),
		}
		// The rotated-out value keeps working on its own route until the
		// grace window closes, so existing edges migrate without downtime.
		previous := strings.TrimSpace(credential.Previous)
		if previous == "" || credential.PreviousExpires.IsZero() || !now().Before(credential.PreviousExpires) {
			continue
		}
		byHash[sha256.Sum256([]byte(previous))] = tokenState{
			id:       credential.ID,
			token:    previous,
			disabled: credential.Disabled,
			legacy:   true,
			expires:  credential.PreviousExpires,
			route:    protocol.RouteForToken(previous),
		}
	}
	s.mu.Lock()
	s.byHash = byHash
	s.mu.Unlock()
}

// lookup resolves a presented bearer value. Comparing SHA-256 digests keeps
// the comparison independent of the provided value's length or content.
// Expired grace-window values are treated as unknown.
func (s *tokenStore) lookup(value string) (tokenState, bool) {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	s.mu.RLock()
	state, ok := s.byHash[digest]
	now := s.now
	s.mu.RUnlock()
	if !ok {
		return tokenState{}, false
	}
	if state.expired(now()) && state.legacy {
		return tokenState{}, false
	}
	return state, true
}

// activeIDs returns the ids of all currently enabled tokens.
func (s *tokenStore) activeIDs() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make(map[string]bool, len(s.byHash))
	for _, state := range s.byHash {
		if !state.disabled && !state.legacy && !state.expired(s.now()) {
			ids[state.id] = true
		}
	}
	return ids
}

func (st tokenState) expired(now time.Time) bool {
	return !st.expires.IsZero() && !now.Before(st.expires)
}

func (st tokenState) lifetimeExpired(now time.Time) bool {
	return !st.legacy && st.expired(now)
}

// legacyExpired reports whether a token's grace window has closed (or was
// never open), so participants still using the old value must be dropped.
func (s *tokenStore) legacyExpired(tokenID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, state := range s.byHash {
		if state.id == tokenID && state.legacy {
			return state.expired(s.now())
		}
	}
	return true
}

// expiredIDs returns token ids whose current value has passed ExpiresAt.
func (s *tokenStore) expiredIDs() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make(map[string]bool)
	now := s.now()
	for _, state := range s.byHash {
		if state.lifetimeExpired(now) {
			ids[state.id] = true
		}
	}
	return ids
}
