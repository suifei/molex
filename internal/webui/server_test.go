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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/suifei/molex/internal/config"
	"github.com/suifei/molex/internal/service"
)

const testPassword = "correct-horse-battery-staple"

func TestAuthenticationAndCSRF(t *testing.T) {
	server := newTestServer(t)
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
	response.Body.Close()

	cfg := config.Default()
	response = doRequest(t, client, http.MethodPost, httpServer.URL+"/api/config/validate", httpServer.URL, cfg)
	assertStatus(t, response, http.StatusForbidden)
	response.Body.Close()

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/config/validate", httpServer.URL, csrf, cfg)
	assertStatus(t, response, http.StatusOK)
	var validation validationResult
	decodeResponse(t, response, &validation)
	if validation.Valid || len(validation.Errors) == 0 {
		t.Fatal("expected the default client configuration to require a secret")
	}

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/secret", "https://attacker.example", csrf, map[string]string{})
	assertStatus(t, response, http.StatusForbidden)
	response.Body.Close()

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/logout", httpServer.URL, csrf, map[string]string{})
	assertStatus(t, response, http.StatusNoContent)
	response.Body.Close()
	response = doRequest(t, client, http.MethodGet, httpServer.URL+"/api/config", "", nil)
	assertStatus(t, response, http.StatusUnauthorized)
	response.Body.Close()
}

func TestRuntimeLifecycle(t *testing.T) {
	server := newTestServer(t)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)
	csrf := login(t, client, httpServer.URL)

	cfg := config.Config{Mode: config.ModeRelay, Listen: "127.0.0.1:0", Tunnel: config.TunnelConfig{}}
	response := doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/runtime/start", httpServer.URL, csrf, cfg)
	assertStatus(t, response, http.StatusAccepted)
	response.Body.Close()
	waitForState(t, client, httpServer.URL, "running")

	response = doAuthorizedRequest(t, client, http.MethodPut, httpServer.URL+"/api/config", httpServer.URL, csrf, cfg)
	assertStatus(t, response, http.StatusConflict)
	response.Body.Close()

	response = doAuthorizedRequest(t, client, http.MethodPost, httpServer.URL+"/api/runtime/stop", httpServer.URL, csrf, map[string]string{})
	assertStatus(t, response, http.StatusOK)
	response.Body.Close()
	waitForState(t, client, httpServer.URL, "idle")
}

func TestConfigCRUDPreservesMultipleSameNameRules(t *testing.T) {
	server := newTestServer(t)
	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	client := newTestClient(t)
	csrf := login(t, client, httpServer.URL)

	cfg := config.Config{
		Mode:   config.ModePunch,
		Role:   config.RoleEdge,
		Secret: "0123456789abcdef0123456789abcdef",
		Token:  "relay-token-for-webui-test",
		Remote: "wss://relay.example/ws/session",
		Tunnel: config.TunnelConfig{Rules: []config.TunnelRule{
			{Name: "shared-edge", Listen: "127.0.0.1:2201", Remote: "ssh"},
			{Name: "shared-edge", Listen: "127.0.0.1:2202", Remote: "rdp"},
		}},
	}
	response := doAuthorizedRequest(t, client, http.MethodPut, httpServer.URL+"/api/config", httpServer.URL, csrf, cfg)
	assertStatus(t, response, http.StatusNoContent)
	response.Body.Close()

	response = doRequest(t, client, http.MethodGet, httpServer.URL+"/api/config", "", nil)
	assertStatus(t, response, http.StatusOK)
	var saved config.Config
	decodeResponse(t, response, &saved)
	if len(saved.Tunnel.Rules) != 2 {
		t.Fatalf("saved rule count = %d, want 2", len(saved.Tunnel.Rules))
	}
	if saved.Tunnel.Rules[0].Name != saved.Tunnel.Rules[1].Name {
		t.Fatalf("same-name rules were changed: %#v", saved.Tunnel.Rules)
	}
}

func TestLoginRateLimit(t *testing.T) {
	server := newTestServer(t)
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
	server := newTestServer(t)
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
	if err := config.Save(configPath, config.Config{Mode: config.ModeRelay, Listen: "127.0.0.1:0", Tunnel: config.TunnelConfig{}}); err != nil {
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

func newTestServer(t *testing.T) *Server {
	t.Helper()
	server, err := New(Options{
		Listen:     "127.0.0.1:0",
		ConfigPath: filepath.Join(t.TempDir(), "molex.json"),
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
