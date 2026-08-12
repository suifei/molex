package webui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/suifei/molex/internal/config"
)

type validationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

type sessionResponse struct {
	Authenticated bool   `json:"authenticated"`
	SetupRequired bool   `json:"setupRequired,omitempty"`
	CSRFToken     string `json:"csrfToken,omitempty"`
}

type loginRequest struct {
	Password string `json:"password"`
}

type setupRequest struct {
	Password string `json:"password"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	session, ok := s.sessionFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusOK, sessionResponse{Authenticated: false, SetupRequired: s.requiresSetup()})
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{Authenticated: true, CSRFToken: session.csrf})
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
	writeJSON(w, http.StatusOK, sessionResponse{Authenticated: true, CSRFToken: session.csrf})
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
	writeJSON(w, http.StatusOK, sessionResponse{Authenticated: true, CSRFToken: session.csrf})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireCSRF(w, r) {
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
		writeJSON(w, http.StatusOK, cfg)
	case http.MethodPut:
		if !requireCSRF(w, r) {
			return
		}
		var cfg config.Config
		if err := decodeJSON(r, &cfg, 1<<20); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cfg = cfg.Normalized()
		if err := cfg.Validate(); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.actionMu.Lock()
		defer s.actionMu.Unlock()
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

func (s *Server) handleValidateConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireCSRF(w, r) {
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
	if !requireCSRF(w, r) {
		return
	}
	var cfg config.Config
	if err := decodeJSON(r, &cfg, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.actionMu.Lock()
	defer s.actionMu.Unlock()
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
	if !requireCSRF(w, r) {
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
	session, _ := sessionFromContext(r.Context())
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
			if !s.sessions.valid(session.key) {
				return
			}
			if _, err := io.WriteString(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *Server) handleGenerateSecret(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	if !requireCSRF(w, r) {
		return
	}
	secret, err := config.GenerateSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret})
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
