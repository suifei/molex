package webui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/suifei/molex/internal/config"
	"github.com/suifei/molex/internal/telemetry"
)

type validationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

type sessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	SetupRequired bool   `json:"setupRequired,omitempty"`
	CSRFToken     string `json:"csrfToken,omitempty"`
	Mode          string `json:"mode"`
	ModeLocked    bool   `json:"modeLocked"`
	AuthRequired  bool   `json:"authRequired"`
}

type loginRequest struct {
	Password string `json:"password"`
}

type setupRequest struct {
	Password string `json:"password"`
}

type bootstrapRequest struct {
	Mode string `json:"mode"`
}

type tokenCreateRequest struct {
	Note     string `json:"note"`
	Lifetime string `json:"lifetime"`
}

type tokenUpdateRequest struct {
	Note     *string `json:"note"`
	Disabled *bool   `json:"disabled"`
	Lifetime *string `json:"lifetime"`
}

type tokenRotateRequest struct {
	GraceDays int `json:"graceDays"`
}

type disconnectRequest struct {
	ID string `json:"id"`
}

type freePortRequest struct {
	LAN bool `json:"lan"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	mode := s.consoleMode()
	if !s.relayConsole() {
		writeJSON(w, http.StatusOK, sessionResponse{
			Authenticated: true,
			CSRFToken:     s.bootCSRF,
			Mode:          mode,
			ModeLocked:    s.consoleModeLocked(),
			AuthRequired:  false,
		})
		return
	}
	session, ok := s.sessionFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusOK, sessionResponse{
			Authenticated: false,
			SetupRequired: s.requiresSetup(),
			Mode:          mode,
			ModeLocked:    true,
			AuthRequired:  true,
		})
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{
		Authenticated: true,
		CSRFToken:     session.csrf,
		Mode:          mode,
		ModeLocked:    true,
		AuthRequired:  true,
	})
}

// handleBootstrap locks a fresh console to the target or edge role chosen in
// the browser. Relay consoles are created from the CLI so their password
// requirement is never skipped.
func (s *Server) handleBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if s.relayConsole() {
		writeError(w, http.StatusConflict, "the console role is already configured")
		return
	}
	if !s.requireMutation(w, r) {
		return
	}
	var input bootstrapRequest
	if err := decodeJSON(r, &input, 4096); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.lockConsoleMode(strings.ToLower(strings.TrimSpace(input.Mode))); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{
		Authenticated: true,
		CSRFToken:     s.bootCSRF,
		Mode:          s.consoleMode(),
		ModeLocked:    true,
		AuthRequired:  false,
	})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	var input setupRequest
	if err := decodeJSON(r, &input, 4096); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(input.Password) < 12 {
		writeError(w, http.StatusBadRequest, "web password must contain at least 12 characters")
		return
	}
	if len(input.Password) > 1024 {
		writeError(w, http.StatusBadRequest, "web password must contain at most 1024 characters")
		return
	}

	s.authMu.Lock()
	if !s.setupPending {
		s.authMu.Unlock()
		writeError(w, http.StatusConflict, "first-run setup is already complete")
		return
	}
	if err := writePrivateFile(s.options.SetupPasswordPath, []byte(input.Password+"\n")); err != nil {
		s.authMu.Unlock()
		writeError(w, http.StatusInternalServerError, "could not save the management password")
		return
	}
	s.passwordHash = hashToken(input.Password)
	s.setupPending = false
	s.authMu.Unlock()

	token, session, err := s.sessions.create(s.options.SessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	setSessionCookie(w, r, token, session.expires, s.options.SessionTTL)
	writeJSON(w, http.StatusOK, sessionResponse{Authenticated: true, CSRFToken: session.csrf, Mode: s.consoleMode(), ModeLocked: true, AuthRequired: true})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "cross-origin request rejected")
		return
	}
	client := clientAddress(r)
	allowed, retryAfter := s.loginLimiter.allowed(client)
	if !allowed {
		seconds := int((retryAfter + time.Second - 1) / time.Second)
		w.Header().Set("Retry-After", strconv.Itoa(max(seconds, 1)))
		writeError(w, http.StatusTooManyRequests, "too many login attempts; try again later")
		return
	}

	var input loginRequest
	if err := decodeJSON(r, &input, 4096); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePasswordInput(input.Password); err != nil || !s.passwordMatches(input.Password) {
		s.loginLimiter.failure(client)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, session, err := s.sessions.create(s.options.SessionTTL)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	s.loginLimiter.success(client)
	setSessionCookie(w, r, token, session.expires, s.options.SessionTTL)
	writeJSON(w, http.StatusOK, sessionResponse{Authenticated: true, CSRFToken: session.csrf, Mode: s.consoleMode(), ModeLocked: true, AuthRequired: true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.requireMutation(w, r) {
		return
	}
	session, _ := sessionFromContext(r.Context())
	s.sessions.delete(session.key)
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := config.LoadOptional(s.options.ConfigPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if cfg.Mode != s.consoleMode() {
			cfg.Mode = s.consoleMode()
		}
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPut:
		if !s.requireMutation(w, r) {
			return
		}
		var cfg config.Config
		if err := decodeJSON(r, &cfg, 1<<20); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cfg = cfg.Normalized()
		if cfg.Mode != s.consoleMode() {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("this console manages a %q configuration; recreate the file with `molex config init` to change roles", s.consoleMode()))
			return
		}
		s.actionMu.Lock()
		defer s.actionMu.Unlock()
		cfg = s.mergeManagedLists(cfg)
		if err := cfg.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if s.manager.Running() {
			writeError(w, http.StatusConflict, "stop MoleX before changing its configuration")
			return
		}
		if err := config.Save(s.options.ConfigPath, cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

// mergeManagedLists keeps the list sections owned by their dedicated
// endpoints (/api/tokens, /api/services, /api/mappings) authoritative on
// disk, so a stale browser configuration can never wipe them through the
// generic save or start endpoints. Callers must hold actionMu.
func (s *Server) mergeManagedLists(cfg config.Config) config.Config {
	current, err := config.LoadOptional(s.options.ConfigPath)
	if err != nil {
		return cfg
	}
	switch s.consoleMode() {
	case config.ModeRelay:
		cfg.Tokens = current.Tokens
	case config.ModeTarget:
		cfg.Services = current.Services
	case config.ModeEdge:
		cfg.Mappings = current.Mappings
	}
	return cfg
}

func (s *Server) handleValidateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.requireMutation(w, r) {
		return
	}
	var cfg config.Config
	if err := decodeJSON(r, &cfg, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := cfg.Validate(); err != nil {
		writeJSON(w, http.StatusOK, validationResult{Valid: false, Errors: strings.Split(err.Error(), "; ")})
		return
	}
	writeJSON(w, http.StatusOK, validationResult{Valid: true, Errors: []string{}})
}

func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.requireMutation(w, r) {
		return
	}
	var cfg config.Config
	if err := decodeJSON(r, &cfg, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg = cfg.Normalized()
	if cfg.Mode != s.consoleMode() {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("this console manages a %q configuration; recreate the file with `molex config init` to change roles", s.consoleMode()))
		return
	}

	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	cfg = s.mergeManagedLists(cfg)
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.manager.Running() {
		writeError(w, http.StatusConflict, "MoleX is already running")
		return
	}
	if err := config.Save(s.options.ConfigPath, cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.manager.Start(cfg); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, s.manager.Status())
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.requireMutation(w, r) {
		return
	}
	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.manager.Stop(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.manager.Status())
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, s.manager.Status())
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, s.manager.Events())
}

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "event streaming is unavailable")
		return
	}
	session, hasSession := sessionFromContext(r.Context())
	events, unsubscribe := s.subscribe()
	defer unsubscribe()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if s.relayConsole() && hasSession && !s.sessions.valid(session.key) {
				return
			}
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// handleTokens lists and creates relay access tokens. Token values stay
// visible to the authenticated relay operator by design (decision 2 of the
// v2 plan); the UI masks them until the operator reveals or copies them.
func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := config.LoadOptional(s.options.ConfigPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		tokens := cfg.Tokens
		if tokens == nil {
			tokens = []config.TokenEntry{}
		}
		writeJSON(w, http.StatusOK, tokens)
	case http.MethodPost:
		if !s.requireMutation(w, r) {
			return
		}
		var input tokenCreateRequest
		if err := decodeJSON(r, &input, 4096); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		value, err := config.GenerateToken()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		id, err := config.GenerateID("tok")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		expiresAt, err := config.ParseLifetime(input.Lifetime, time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		entry := config.TokenEntry{
			ID:        id,
			Token:     value,
			Note:      strings.TrimSpace(input.Note),
			CreatedAt: time.Now().UTC(),
			ExpiresAt: expiresAt,
		}

		s.actionMu.Lock()
		defer s.actionMu.Unlock()
		cfg, err := config.LoadOptional(s.options.ConfigPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		cfg.Mode = config.ModeRelay
		cfg.Tokens = append(cfg.Tokens, entry)
		if err := cfg.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := config.Save(s.options.ConfigPath, cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.manager.UpdateTokens(cfg.Tokens)
		message := fmt.Sprintf("Token %s was created", entry.ID)
		if !entry.ExpiresAt.IsZero() {
			message = fmt.Sprintf("Token %s was created; it expires at %s", entry.ID, entry.ExpiresAt.Format(time.RFC3339))
		}
		s.manager.RecordAudit(telemetry.Event{
			Type:    "token_created",
			Level:   "info",
			Message: message,
		})
		writeJSON(w, http.StatusCreated, entry)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleTokenItem(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/tokens/")
	if rotateID, ok := strings.CutSuffix(id, "/rotate"); ok {
		s.handleTokenRotate(w, r, rotateID)
		return
	}
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	switch r.Method {
	case http.MethodPut:
		if !s.requireMutation(w, r) {
			return
		}
		var input tokenUpdateRequest
		if err := decodeJSON(r, &input, 4096); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.actionMu.Lock()
		defer s.actionMu.Unlock()
		cfg, err := config.LoadOptional(s.options.ConfigPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		index := -1
		for i, token := range cfg.Tokens {
			if token.ID == id {
				index = i
				break
			}
		}
		if index < 0 {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		if input.Note != nil {
			cfg.Tokens[index].Note = strings.TrimSpace(*input.Note)
		}
		if input.Disabled != nil {
			cfg.Tokens[index].Disabled = *input.Disabled
		}
		if input.Lifetime != nil {
			expiresAt, err := config.ParseLifetime(*input.Lifetime, time.Now().UTC())
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			cfg.Tokens[index].ExpiresAt = expiresAt
		}
		if err := cfg.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := config.Save(s.options.ConfigPath, cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.manager.UpdateTokens(cfg.Tokens)
		if input.Disabled != nil {
			action := "token_enabled"
			if *input.Disabled {
				action = "token_disabled"
			}
			s.manager.RecordAudit(telemetry.Event{
				Type:    action,
				Level:   "warning",
				Message: fmt.Sprintf("Token %s was %s", id, strings.TrimPrefix(action, "token_")),
			})
		}
		if input.Lifetime != nil {
			message := fmt.Sprintf("Token %s lifetime is now unlimited", id)
			if !cfg.Tokens[index].ExpiresAt.IsZero() {
				message = fmt.Sprintf("Token %s lifetime now expires at %s", id, cfg.Tokens[index].ExpiresAt.Format(time.RFC3339))
			}
			s.manager.RecordAudit(telemetry.Event{
				Type:    "token_lifetime_updated",
				Level:   "warning",
				Message: message,
			})
		}
		writeJSON(w, http.StatusOK, cfg.Tokens[index])
	case http.MethodDelete:
		if !s.requireMutation(w, r) {
			return
		}
		s.actionMu.Lock()
		defer s.actionMu.Unlock()
		cfg, err := config.LoadOptional(s.options.ConfigPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		kept := cfg.Tokens[:0]
		found := false
		for _, token := range cfg.Tokens {
			if token.ID == id {
				found = true
				continue
			}
			kept = append(kept, token)
		}
		if !found {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		cfg.Tokens = kept
		if err := config.Save(s.options.ConfigPath, cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.manager.UpdateTokens(cfg.Tokens)
		s.manager.RecordAudit(telemetry.Event{
			Type:    "token_deleted",
			Level:   "warning",
			Message: fmt.Sprintf("Token %s was deleted", id),
		})
		w.WriteHeader(http.StatusNoContent)
	default:
		methodNotAllowed(w, http.MethodPut, http.MethodDelete)
	}
}

// handleTokenRotate issues a fresh token value while keeping the previous
// one valid through a grace window, so a group migrates without downtime.
func (s *Server) handleTokenRotate(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	if !s.requireMutation(w, r) {
		return
	}
	var input tokenRotateRequest
	if err := decodeJSON(r, &input, 4096); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	graceDays := input.GraceDays
	if graceDays == 0 {
		graceDays = 3
	}
	if graceDays < 1 || graceDays > 30 {
		writeError(w, http.StatusBadRequest, "graceDays must be between 1 and 30")
		return
	}
	value, err := config.GenerateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.actionMu.Lock()
	defer s.actionMu.Unlock()
	cfg, err := config.LoadOptional(s.options.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	index := -1
	for i, token := range cfg.Tokens {
		if token.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	expires := time.Now().UTC().Add(time.Duration(graceDays) * 24 * time.Hour)
	cfg.Tokens[index].PreviousToken = cfg.Tokens[index].Token
	cfg.Tokens[index].PreviousExpiresAt = expires
	cfg.Tokens[index].Token = value
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := config.Save(s.options.ConfigPath, cfg); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.manager.UpdateTokens(cfg.Tokens)
	s.manager.RecordAudit(telemetry.Event{
		Type:    "token_rotated",
		Level:   "warning",
		Message: fmt.Sprintf("Token %s was rotated; the previous value stays valid until %s", id, expires.Format(time.RFC3339)),
	})
	writeJSON(w, http.StatusOK, cfg.Tokens[index])
}

func (s *Server) handleDisconnectPeer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.requireMutation(w, r) {
		return
	}
	var input disconnectRequest
	if err := decodeJSON(r, &input, 4096); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(input.ID) == "" {
		writeError(w, http.StatusBadRequest, "peer id is required")
		return
	}
	if !s.manager.DisconnectPeer(strings.TrimSpace(input.ID)) {
		writeError(w, http.StatusNotFound, "peer not found or the relay is not running")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleServices replaces the target's published service list and applies
// it to the running client immediately.
func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := config.LoadOptional(s.options.ConfigPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		services := cfg.Services
		if services == nil {
			services = []config.ServiceEntry{}
		}
		writeJSON(w, http.StatusOK, services)
	case http.MethodPut:
		if !s.requireMutation(w, r) {
			return
		}
		var services []config.ServiceEntry
		if err := decodeJSON(r, &services, 1<<20); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		for index := range services {
			if strings.TrimSpace(services[index].ID) == "" {
				id, err := config.GenerateID("svc")
				if err != nil {
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				services[index].ID = id
			}
		}

		s.actionMu.Lock()
		defer s.actionMu.Unlock()
		cfg, err := config.LoadOptional(s.options.ConfigPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		cfg.Mode = config.ModeTarget
		cfg.Services = services
		cfg = cfg.Normalized()
		if err := cfg.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := config.Save(s.options.ConfigPath, cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.manager.UpdateServices(cfg.Services)
		writeJSON(w, http.StatusOK, cfg.Services)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

// handleMappings replaces the edge's local mappings and applies them to the
// running client immediately.
func (s *Server) handleMappings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := config.LoadOptional(s.options.ConfigPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		mappings := cfg.Mappings
		if mappings == nil {
			mappings = []config.MappingEntry{}
		}
		writeJSON(w, http.StatusOK, mappings)
	case http.MethodPut:
		if !s.requireMutation(w, r) {
			return
		}
		var mappings []config.MappingEntry
		if err := decodeJSON(r, &mappings, 1<<20); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		s.actionMu.Lock()
		defer s.actionMu.Unlock()
		cfg, err := config.LoadOptional(s.options.ConfigPath)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		cfg.Mode = config.ModeEdge
		cfg.Mappings = mappings
		cfg = cfg.Normalized()
		if err := cfg.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := config.Save(s.options.ConfigPath, cfg); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.manager.UpdateMappings(cfg.Mappings)
		writeJSON(w, http.StatusOK, cfg.Mappings)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

// handleFreePort suggests an available local port for a new mapping.
func (s *Server) handleFreePort(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !s.requireMutation(w, r) {
		return
	}
	var input freePortRequest
	if err := decodeJSON(r, &input, 4096); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	host := "127.0.0.1"
	if input.LAN {
		host = "0.0.0.0"
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not find a free local port: "+err.Error())
		return
	}
	address := listener.Addr().String()
	_ = listener.Close()
	_, portText, err := net.SplitHostPort(address)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not find a free local port")
		return
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not find a free local port")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"port": port})
}

func decodeJSON(r *http.Request, destination any, limit int64) error {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return errors.New("Content-Type must be application/json")
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, limit+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request must contain exactly one JSON object")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writePrivateFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".molex-credential-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}
