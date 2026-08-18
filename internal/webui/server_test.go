package webui

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/suifei/molex/internal/config"
	"github.com/suifei/molex/internal/service"
)

const testPassword = "correct-horse-battery-staple"

func TestRelayConsoleAuthenticationAndCSRF(t *testing.T) {
	server := newRelayTestServer(t)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)

	response := doRequest(t, client, http.MethodGet, httpServer.URL+"/api/config", "", nil)
	assertStatus(t, response, http.StatusUnauthorized)
	if response.Header.Get("Content-Security-Policy") == "" || response.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatal("security headers are missing")
	}
	response.Body.Close()

	response = doRequest(t, client, http.MethodPost, httpServer.URL+"/api/login", httpServer.URL, map[string]string{"password": "wrong-password"})
	assertStatus(t, response, http.StatusUnauthorized)
	response.Body.Close()

	csrf := login(t, client, httpServer.URL)
	response = doRequest(t, client, http.MethodGet, httpServer.URL+"/api/config", "", nil)
	assertStatus(t, response, http.StatusOK)
	var served config.Config
	decodeResponse(t, response, &served)
	if served.Mode != config.ModeRelay {
		t.Fatalf("relay console served mode %q", served.Mode)
	}

	invalid := config.Config{Mode: config.ModeRelay, Listen: "bad-address"}
	response = doRequest(t, client, http.MethodPost, httpServer.URL+"/api/config/validate", httpServer.URL, invalid)
	assertStatus(t, response, http.StatusForbidden)
	response.Body.Close()

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/config/validate", httpServer.URL, csrf, invalid)
	assertStatus(t, response, http.StatusOK)
	var validation validationResult
	decodeResponse(t, response, &validation)
	if validation.Valid || len(validation.Errors) == 0 {
		t.Fatal("expected the invalid relay configuration to be rejected")
	}

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/tokens", "https://attacker.example", csrf, tokenCreateRequest{Note: "x"})
	assertStatus(t, response, http.StatusForbidden)
	response.Body.Close()

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/logout", httpServer.URL, csrf, map[string]string{})
	assertStatus(t, response, http.StatusNoContent)
	response.Body.Close()
	response = doRequest(t, client, http.MethodGet, httpServer.URL+"/api/config", "", nil)
	assertStatus(t, response, http.StatusUnauthorized)
	response.Body.Close()
}

func TestRelayTokenCRUDPersistsAndReturnsFullValues(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "molex.json")
	server := newRelayTestServerAt(t, configPath)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)
	csrf := login(t, client, httpServer.URL)

	response := doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/tokens", httpServer.URL, csrf, tokenCreateRequest{Note: "office"})
	assertStatus(t, response, http.StatusCreated)
	var created config.TokenEntry
	decodeResponse(t, response, &created)
	if created.ID == "" || !strings.HasPrefix(created.Token, "mx2_") || created.Note != "office" || created.CreatedAt.IsZero() {
		t.Fatalf("created token = %#v", created)
	}

	response = doRequest(t, client, http.MethodGet, httpServer.URL+"/api/tokens", "", nil)
	assertStatus(t, response, http.StatusOK)
	var listed []config.TokenEntry
	decodeResponse(t, response, &listed)
	if len(listed) != 1 || listed[0].Token != created.Token {
		t.Fatalf("token list = %#v", listed)
	}

	disabled := true
	response = doAuthorizedRequest(t, client, http.MethodPut, httpServer.URL+"/api/tokens/"+created.ID, httpServer.URL, csrf, tokenUpdateRequest{Disabled: &disabled})
	assertStatus(t, response, http.StatusOK)
	var updated config.TokenEntry
	decodeResponse(t, response, &updated)
	if !updated.Disabled {
		t.Fatalf("updated token = %#v, want disabled", updated)
	}

	saved, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Mode != config.ModeRelay || len(saved.Tokens) != 1 || !saved.Tokens[0].Disabled {
		t.Fatalf("persisted config = %#v", saved)
	}

	response = doAuthorizedRequest(t, client, http.MethodDelete, httpServer.URL+"/api/tokens/"+created.ID, httpServer.URL, csrf, nil)
	assertStatus(t, response, http.StatusNoContent)
	response.Body.Close()
	response = doAuthorizedRequest(t, client, http.MethodDelete, httpServer.URL+"/api/tokens/"+created.ID, httpServer.URL, csrf, nil)
	assertStatus(t, response, http.StatusNotFound)
	response.Body.Close()
}

func TestRelayTokenLifetimeCreateUpdateAndClear(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "molex.json")
	server := newRelayTestServerAt(t, configPath)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)
	csrf := login(t, client, httpServer.URL)

	response := doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/tokens", httpServer.URL, csrf, tokenCreateRequest{Note: "lab", Lifetime: "30d"})
	assertStatus(t, response, http.StatusCreated)
	var created config.TokenEntry
	decodeResponse(t, response, &created)
	if created.ExpiresAt.IsZero() || created.ExpiresAt.Before(time.Now().Add(20*24*time.Hour)) {
		t.Fatalf("created expiry = %s, want about 30 days", created.ExpiresAt)
	}

	lifetime := "never"
	response = doAuthorizedRequest(t, client, http.MethodPut, httpServer.URL+"/api/tokens/"+created.ID, httpServer.URL, csrf, tokenUpdateRequest{Lifetime: &lifetime})
	assertStatus(t, response, http.StatusOK)
	var cleared config.TokenEntry
	decodeResponse(t, response, &cleared)
	if !cleared.ExpiresAt.IsZero() {
		t.Fatalf("cleared expiry = %s, want unlimited", cleared.ExpiresAt)
	}

	lifetime = "7d"
	response = doAuthorizedRequest(t, client, http.MethodPut, httpServer.URL+"/api/tokens/"+created.ID, httpServer.URL, csrf, tokenUpdateRequest{Lifetime: &lifetime})
	assertStatus(t, response, http.StatusOK)
	var week config.TokenEntry
	decodeResponse(t, response, &week)
	if week.ExpiresAt.IsZero() || week.ExpiresAt.Before(time.Now().Add(5*24*time.Hour)) {
		t.Fatalf("updated expiry = %s, want about 7 days", week.ExpiresAt)
	}

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/tokens", httpServer.URL, csrf, tokenCreateRequest{Lifetime: "2h"})
	assertStatus(t, response, http.StatusBadRequest)
	response.Body.Close()
}

func TestRelayTokenRotateKeepsPreviousValueThroughGrace(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "molex.json")
	server := newRelayTestServerAt(t, configPath)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)
	csrf := login(t, client, httpServer.URL)

	response := doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/tokens", httpServer.URL, csrf, tokenCreateRequest{Note: "office"})
	assertStatus(t, response, http.StatusCreated)
	var created config.TokenEntry
	decodeResponse(t, response, &created)

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/tokens/"+created.ID+"/rotate", httpServer.URL, csrf, tokenRotateRequest{GraceDays: 2})
	assertStatus(t, response, http.StatusOK)
	var rotated config.TokenEntry
	decodeResponse(t, response, &rotated)
	if rotated.Token == "" || rotated.Token == created.Token {
		t.Fatalf("rotated token reused the previous value: %#v", rotated)
	}
	if rotated.PreviousToken != created.Token {
		t.Fatalf("previous token = %q, want %q", rotated.PreviousToken, created.Token)
	}
	if rotated.PreviousExpiresAt.IsZero() || rotated.PreviousExpiresAt.Before(time.Now().Add(24*time.Hour)) {
		t.Fatalf("grace expiry = %s, want about two days", rotated.PreviousExpiresAt)
	}

	saved, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Tokens[0].Token != rotated.Token || saved.Tokens[0].PreviousToken != created.Token {
		t.Fatalf("persisted rotation = %#v", saved.Tokens[0])
	}
}

func TestRelayRuntimeLifecycleAndTokenUpdateWhileRunning(t *testing.T) {
	server := newRelayTestServer(t)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)
	csrf := login(t, client, httpServer.URL)

	cfg := config.Config{Mode: config.ModeRelay, Listen: "127.0.0.1:0"}
	response := doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/runtime/start", httpServer.URL, csrf, cfg)
	assertStatus(t, response, http.StatusAccepted)
	response.Body.Close()
	waitForState(t, client, httpServer.URL, "running")

	response = doAuthorizedRequest(t, client, http.MethodPut, httpServer.URL+"/api/config", httpServer.URL, csrf, cfg)
	assertStatus(t, response, http.StatusConflict)
	response.Body.Close()

	// Token management stays available while the relay is running.
	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/tokens", httpServer.URL, csrf, tokenCreateRequest{Note: "live"})
	assertStatus(t, response, http.StatusCreated)
	response.Body.Close()

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/peers/disconnect", httpServer.URL, csrf, disconnectRequest{ID: "999"})
	assertStatus(t, response, http.StatusNotFound)
	response.Body.Close()

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/runtime/stop", httpServer.URL, csrf, map[string]string{})
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()
	waitForState(t, client, httpServer.URL, "idle")
}

func TestStartAndSaveKeepManagedListsAuthoritative(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "molex.json")
	server := newRelayTestServerAt(t, configPath)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)
	csrf := login(t, client, httpServer.URL)

	response := doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/tokens", httpServer.URL, csrf, tokenCreateRequest{Note: "keep-me"})
	assertStatus(t, response, http.StatusCreated)
	response.Body.Close()

	// A browser that loaded its configuration before the token was created
	// starts the runtime with an empty token list; the server must keep the
	// on-disk tokens instead of wiping them.
	stale := config.Config{Mode: config.ModeRelay, Listen: "127.0.0.1:0"}
	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/runtime/start", httpServer.URL, csrf, stale)
	assertStatus(t, response, http.StatusAccepted)
	response.Body.Close()
	waitForState(t, client, httpServer.URL, "running")
	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/runtime/stop", httpServer.URL, csrf, map[string]string{})
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()

	response = doAuthorizedRequest(t, client, http.MethodPut, httpServer.URL+"/api/config", httpServer.URL, csrf, stale)
	assertStatus(t, response, http.StatusNoContent)
	response.Body.Close()

	saved, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Tokens) != 1 || saved.Tokens[0].Note != "keep-me" {
		t.Fatalf("tokens after stale start/save = %#v, want the created token preserved", saved.Tokens)
	}
}

func TestRelayConsoleRejectsForeignModeConfig(t *testing.T) {
	server := newRelayTestServer(t)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)
	csrf := login(t, client, httpServer.URL)

	edgeConfig := config.Config{Mode: config.ModeEdge, Remote: "wss://relay.example.com", Token: "mx2_0123456789abcdef"}
	response := doAuthorizedRequest(t, client, http.MethodPut, httpServer.URL+"/api/config", httpServer.URL, csrf, edgeConfig)
	assertStatus(t, response, http.StatusBadRequest)
	response.Body.Close()

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/runtime/start", httpServer.URL, csrf, edgeConfig)
	assertStatus(t, response, http.StatusBadRequest)
	response.Body.Close()
}

func TestEdgeConsoleWorksWithoutLogin(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "molex.json")
	if err := config.Save(configPath, config.Config{
		Mode:   config.ModeEdge,
		Remote: "wss://relay.example.com/ws/session",
		Token:  "mx2_0123456789abcdef",
	}); err != nil {
		t.Fatal(err)
	}
	server, err := New(Options{
		Listen:     "127.0.0.1:0",
		ConfigPath: configPath,
		Mode:       config.ModeEdge,
		ModeLocked: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)

	response := doRequest(t, client, http.MethodGet, httpServer.URL+"/api/session", "", nil)
	assertStatus(t, response, http.StatusOK)
	var session sessionResponse
	decodeResponse(t, response, &session)
	if !session.Authenticated || session.AuthRequired || session.Mode != config.ModeEdge || session.CSRFToken == "" {
		t.Fatalf("edge session = %#v", session)
	}

	// Reads work without any cookie or password.
	response = doRequest(t, client, http.MethodGet, httpServer.URL+"/api/config", "", nil)
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()

	// Mutations still require the per-boot CSRF token.
	mappings := []config.MappingEntry{{Service: "svc-1", Port: 28080}}
	response = doRequest(t, client, http.MethodPut, httpServer.URL+"/api/mappings", httpServer.URL, mappings)
	assertStatus(t, response, http.StatusForbidden)
	response.Body.Close()

	response = doAuthorizedRequest(t, client, http.MethodPut, httpServer.URL+"/api/mappings", httpServer.URL, session.CSRFToken, mappings)
	assertStatus(t, response, http.StatusOK)
	var savedMappings []config.MappingEntry
	decodeResponse(t, response, &savedMappings)
	if len(savedMappings) != 1 || savedMappings[0].Service != "svc-1" {
		t.Fatalf("saved mappings = %#v", savedMappings)
	}
	saved, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Mode != config.ModeEdge || len(saved.Mappings) != 1 {
		t.Fatalf("persisted edge config = %#v", saved)
	}

	// A free-port suggestion returns a usable loopback port.
	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/ports/free", httpServer.URL, session.CSRFToken, freePortRequest{})
	assertStatus(t, response, http.StatusOK)
	var portResult map[string]int
	decodeResponse(t, response, &portResult)
	if portResult["port"] < 1 || portResult["port"] > 65535 {
		t.Fatalf("suggested port = %#v", portResult)
	}

	// Relay-only and target-only endpoints stay hidden.
	response = doRequest(t, client, http.MethodGet, httpServer.URL+"/api/tokens", "", nil)
	assertStatus(t, response, http.StatusNotFound)
	response.Body.Close()
	response = doRequest(t, client, http.MethodGet, httpServer.URL+"/api/services", "", nil)
	assertStatus(t, response, http.StatusNotFound)
	response.Body.Close()
	response = doRequest(t, client, http.MethodPost, httpServer.URL+"/api/login", httpServer.URL, map[string]string{"password": "x"})
	assertStatus(t, response, http.StatusNotFound)
	response.Body.Close()
}

func TestTargetConsoleServicesAssignIDsAndPersist(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "molex.json")
	if err := config.Save(configPath, config.Config{
		Mode:   config.ModeTarget,
		Remote: "wss://relay.example.com/ws/session",
		Token:  "mx2_0123456789abcdef",
	}); err != nil {
		t.Fatal(err)
	}
	server, err := New(Options{
		Listen:     "127.0.0.1:0",
		ConfigPath: configPath,
		Mode:       config.ModeTarget,
		ModeLocked: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)
	csrf := localSession(t, client, httpServer.URL)

	services := []config.ServiceEntry{
		{Name: "web", Address: "10.188.200.16:30927"},
		{ID: "svc-fixed", Name: "ssh", Address: "127.0.0.1:22"},
	}
	response := doAuthorizedRequest(t, client, http.MethodPut, httpServer.URL+"/api/services", httpServer.URL, csrf, services)
	assertStatus(t, response, http.StatusOK)
	var savedServices []config.ServiceEntry
	decodeResponse(t, response, &savedServices)
	if len(savedServices) != 2 || savedServices[0].ID == "" || savedServices[1].ID != "svc-fixed" {
		t.Fatalf("saved services = %#v", savedServices)
	}

	saved, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Services) != 2 || saved.Services[0].Name != "web" {
		t.Fatalf("persisted services = %#v", saved.Services)
	}

	// Duplicate names must be rejected with an actionable validation error.
	duplicate := []config.ServiceEntry{
		{Name: "web", Address: "10.0.0.5:80"},
		{Name: "web", Address: "10.0.0.6:80"},
	}
	response = doAuthorizedRequest(t, client, http.MethodPut, httpServer.URL+"/api/services", httpServer.URL, csrf, duplicate)
	assertStatus(t, response, http.StatusBadRequest)
	response.Body.Close()
}

func TestLocalConsoleRejectsRemoteAndRebindingRequests(t *testing.T) {
	server, err := New(Options{
		Listen:     "127.0.0.1:0",
		ConfigPath: filepath.Join(t.TempDir(), "molex.json"),
		Mode:       config.ModeEdge,
		ModeLocked: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	// DNS rebinding: loopback socket but a foreign Host header.
	request := httptest.NewRequest(http.MethodGet, "http://evil.example/api/config", nil)
	request.RemoteAddr = "127.0.0.1:52345"
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("foreign host status = %d, want 403", recorder.Code)
	}

	// Direct remote connection.
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:9090/api/config", nil)
	request.RemoteAddr = "198.51.100.7:40000"
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("remote peer status = %d, want 403", recorder.Code)
	}

	// Cross-origin browser context.
	request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:9090/api/config", nil)
	request.RemoteAddr = "127.0.0.1:52345"
	request.Header.Set("Origin", "https://evil.example")
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d, want 403", recorder.Code)
	}
}

func TestBootstrapLocksConsoleRoleOnce(t *testing.T) {
	server, err := New(Options{
		Listen:     "127.0.0.1:0",
		ConfigPath: filepath.Join(t.TempDir(), "molex.json"),
		Mode:       config.ModeEdge,
		ModeLocked: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)

	csrf := localSession(t, client, httpServer.URL)
	response := doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/bootstrap", httpServer.URL, csrf, bootstrapRequest{Mode: "relay"})
	assertStatus(t, response, http.StatusConflict)
	response.Body.Close()

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/bootstrap", httpServer.URL, csrf, bootstrapRequest{Mode: "target"})
	assertStatus(t, response, http.StatusOK)
	var session sessionResponse
	decodeResponse(t, response, &session)
	if session.Mode != config.ModeTarget || !session.ModeLocked {
		t.Fatalf("bootstrap session = %#v", session)
	}

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/bootstrap", httpServer.URL, csrf, bootstrapRequest{Mode: "edge"})
	assertStatus(t, response, http.StatusConflict)
	response.Body.Close()

	// After bootstrap the console serves target endpoints.
	response = doRequest(t, client, http.MethodGet, httpServer.URL+"/api/services", "", nil)
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()
}

func TestLoginRateLimit(t *testing.T) {
	server := newRelayTestServer(t)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)
	for range maxLoginFailures {
		response := doRequest(t, client, http.MethodPost, httpServer.URL+"/api/login", httpServer.URL, map[string]string{"password": "wrong-password"})
		assertStatus(t, response, http.StatusUnauthorized)
		response.Body.Close()
	}
	response := doRequest(t, client, http.MethodPost, httpServer.URL+"/api/login", httpServer.URL, map[string]string{"password": testPassword})
	assertStatus(t, response, http.StatusTooManyRequests)
	if response.Header.Get("Retry-After") == "" {
		t.Fatal("rate-limited response is missing Retry-After")
	}
	response.Body.Close()
}

func TestStaticAssetsAndHealth(t *testing.T) {
	server := newRelayTestServer(t)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)

	response := doRequest(t, client, http.MethodGet, httpServer.URL+"/healthz", "", nil)
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()

	response = doRequest(t, client, http.MethodGet, httpServer.URL+"/", "", nil)
	assertStatus(t, response, http.StatusOK)
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || !bytes.Contains(body, []byte("<title>MoleX</title>")) {
		t.Fatal("embedded index was not served")
	}

	response = doRequest(t, client, http.MethodGet, httpServer.URL+"/.keep", "", nil)
	assertStatus(t, response, http.StatusNotFound)
	response.Body.Close()
}

func TestWebServerRejectsUnsafeOptions(t *testing.T) {
	_, err := New(Options{Listen: "0.0.0.0:9090", Password: testPassword})
	if err == nil || !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("expected non-loopback listen rejection, got %v", err)
	}
	_, err = New(Options{Listen: "127.0.0.1:9090", Password: "short"})
	if err == nil || !strings.Contains(err.Error(), "12 characters") {
		t.Fatalf("expected short password rejection, got %v", err)
	}
	// Local consoles do not need a password at all.
	if _, err := New(Options{Listen: "127.0.0.1:9090", Mode: config.ModeEdge, ModeLocked: true}); err != nil {
		t.Fatalf("edge console rejected without password: %v", err)
	}
}

func TestFirstRunSetupCreatesPrivatePasswordAndSession(t *testing.T) {
	directory := t.TempDir()
	passwordPath := filepath.Join(directory, "credentials", "web-password")
	server, err := New(Options{
		Listen:            "127.0.0.1:0",
		ConfigPath:        filepath.Join(directory, "molex.json"),
		SetupPasswordPath: passwordPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)

	response := doRequest(t, client, http.MethodGet, httpServer.URL+"/api/session", "", nil)
	assertStatus(t, response, http.StatusOK)
	var initial sessionResponse
	decodeResponse(t, response, &initial)
	if !initial.SetupRequired || initial.Authenticated {
		t.Fatalf("initial session = %#v, want setup required", initial)
	}

	response = doRequest(t, client, http.MethodPost, httpServer.URL+"/api/setup", httpServer.URL, setupRequest{Password: "short"})
	assertStatus(t, response, http.StatusBadRequest)
	response.Body.Close()

	response = doRequest(t, client, http.MethodPost, httpServer.URL+"/api/setup", httpServer.URL, setupRequest{Password: testPassword})
	assertStatus(t, response, http.StatusOK)
	var setup sessionResponse
	decodeResponse(t, response, &setup)
	if !setup.Authenticated || setup.CSRFToken == "" {
		t.Fatalf("setup session = %#v", setup)
	}
	data, err := os.ReadFile(passwordPath)
	if err != nil || strings.TrimSpace(string(data)) != testPassword {
		t.Fatalf("saved password = %q, %v", data, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(passwordPath)
		if err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("password permissions = %v, %v", info, err)
		}
	}

	response = doRequest(t, client, http.MethodPost, httpServer.URL+"/api/setup", httpServer.URL, setupRequest{Password: "another-secure-password"})
	assertStatus(t, response, http.StatusConflict)
	response.Body.Close()
}

func TestSessionCookieUsesConfiguredClockAndTTL(t *testing.T) {
	fixedNow := time.Date(2040, time.January, 2, 3, 4, 5, 0, time.UTC)
	ttl := 90 * time.Minute
	server, err := New(Options{
		Listen:     "127.0.0.1:0",
		ConfigPath: filepath.Join(t.TempDir(), "molex.json"),
		Password:   testPassword,
		SessionTTL: ttl,
		Now:        func() time.Time { return fixedNow },
	})
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "http://molex.local/api/login", strings.NewReader(`{"password":"`+testPassword+`"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://molex.local")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	response := recorder.Result()
	defer response.Body.Close()
	assertStatus(t, response, http.StatusOK)

	var sessionCookie *http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == sessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("login response did not set a session cookie")
	}
	if sessionCookie.MaxAge != int(ttl/time.Second) {
		t.Fatalf("expected cookie Max-Age %d, got %d", int(ttl/time.Second), sessionCookie.MaxAge)
	}
	if !sessionCookie.Expires.Equal(fixedNow.Add(ttl)) {
		t.Fatalf("expected cookie expiry %s, got %s", fixedNow.Add(ttl), sessionCookie.Expires)
	}
}

func TestRunStopsAutostartWhenWebListenFails(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()

	configPath := filepath.Join(t.TempDir(), "relay.json")
	if err := config.Save(configPath, config.Config{Mode: config.ModeRelay, Listen: "127.0.0.1:0"}); err != nil {
		t.Fatal(err)
	}
	server, err := New(Options{
		Listen:     occupied.Addr().String(),
		ConfigPath: configPath,
		Password:   testPassword,
		AutoStart:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	err = server.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "listen for web console") {
		t.Fatalf("expected web listen failure, got %v", err)
	}
	if server.manager.Running() {
		t.Fatal("autostart runtime remained active after the web listener failed")
	}
}

func TestRunSelectsAvailableLoopbackPort(t *testing.T) {
	ready := make(chan string, 1)
	server, err := New(Options{
		Listen:     "127.0.0.1:0",
		ConfigPath: filepath.Join(t.TempDir(), "molex.json"),
		Password:   testPassword,
		OnReady:    func(address string) { ready <- address },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()

	var address string
	select {
	case address = <-ready:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for WebUI readiness")
	}
	u, err := url.Parse(address)
	if err != nil || u.Hostname() != "127.0.0.1" || u.Port() == "0" || u.Port() == "" {
		t.Fatalf("WebUI selected invalid loopback address %q", address)
	}
	response, err := http.Get(address + "healthz")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("WebUI did not stop")
	}
}

func TestRunAdvancesFromOccupiedDefaultPort(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	occupiedPort := occupied.Addr().(*net.TCPAddr).Port
	if occupiedPort == 65535 {
		t.Skip("cannot test the next port after 65535")
	}
	ready := make(chan string, 1)
	server, err := New(Options{
		Listen:     occupied.Addr().String(),
		AutoListen: true,
		ConfigPath: filepath.Join(t.TempDir(), "molex.json"),
		Password:   testPassword,
		OnReady:    func(address string) { ready <- address },
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx) }()
	select {
	case address := <-ready:
		u, err := url.Parse(address)
		if err != nil || u.Port() == strconv.Itoa(occupiedPort) || u.Port() == "" {
			t.Fatalf("WebUI did not advance from occupied port %d: %q", occupiedPort, address)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for fallback WebUI port")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func newRelayTestServer(t *testing.T) *Server {
	t.Helper()
	return newRelayTestServerAt(t, filepath.Join(t.TempDir(), "molex.json"))
}

func newRelayTestServerAt(t *testing.T, configPath string) *Server {
	t.Helper()
	server, err := New(Options{
		Listen:     "127.0.0.1:0",
		ConfigPath: configPath,
		Password:   testPassword,
	})
	if err != nil {
		t.Fatal(err)
	}
	return server
}

func newTestClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Jar: jar, Timeout: 5 * time.Second}
}

func login(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()
	response := doRequest(t, client, http.MethodPost, baseURL+"/api/login", baseURL, map[string]string{"password": testPassword})
	assertStatus(t, response, http.StatusOK)
	var session sessionResponse
	decodeResponse(t, response, &session)
	if !session.Authenticated || session.CSRFToken == "" {
		t.Fatal("login did not return an authenticated session")
	}
	return session.CSRFToken
}

func localSession(t *testing.T, client *http.Client, baseURL string) string {
	t.Helper()
	response := doRequest(t, client, http.MethodGet, baseURL+"/api/session", "", nil)
	assertStatus(t, response, http.StatusOK)
	var session sessionResponse
	decodeResponse(t, response, &session)
	if !session.Authenticated || session.CSRFToken == "" {
		t.Fatalf("local session = %#v", session)
	}
	return session.CSRFToken
}

func doAuthorizedRequest(t *testing.T, client *http.Client, method, url, origin, csrf string, body any) *http.Response {
	t.Helper()
	request := newRequest(t, method, url, origin, body)
	request.Header.Set("X-MoleX-CSRF", csrf)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func doRequest(t *testing.T, client *http.Client, method, url, origin string, body any) *http.Response {
	t.Helper()
	request := newRequest(t, method, url, origin, body)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func newRequest(t *testing.T, method, url, origin string, body any) *http.Request {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	return request
}

func decodeResponse(t *testing.T, response *http.Response, destination any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatal(err)
	}
}

func assertStatus(t *testing.T, response *http.Response, expected int) {
	t.Helper()
	if response.StatusCode != expected {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		t.Fatalf("expected HTTP %d, got %d: %s", expected, response.StatusCode, body)
	}
}

func waitForState(t *testing.T, client *http.Client, baseURL, expected string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		response := doRequest(t, client, http.MethodGet, baseURL+"/api/runtime/status", "", nil)
		assertStatus(t, response, http.StatusOK)
		var status service.Status
		decodeResponse(t, response, &status)
		if status.State == expected {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("runtime did not reach state %q", expected)
}
