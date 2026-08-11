package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestConfigHasAtMostSevenTopLevelFields(t *testing.T) {
	payload, err := json.Marshal(Default())
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) > 7 {
		t.Fatalf("config has %d top-level fields, want at most 7", len(fields))
	}
}

func TestNormalizeRemote(t *testing.T) {
	tests := map[string]string{
		"molex.example.com:443":        "wss://molex.example.com:443/ws/session",
		"wss://example.com/custom":     "wss://example.com/custom",
		"ws://127.0.0.1:8080":          "ws://127.0.0.1:8080/ws/session",
		"  wss://example.com/session ": "wss://example.com/session",
	}
	for input, expected := range tests {
		actual, err := NormalizeRemote(input)
		if err != nil {
			t.Fatalf("NormalizeRemote(%q): %v", input, err)
		}
		if actual != expected {
			t.Errorf("NormalizeRemote(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestPunchValidationRejectsRemotePlainWebSocket(t *testing.T) {
	cfg := Default()
	cfg.Secret = strings.Repeat("s", 32)
	cfg.Remote = "ws://relay.example.com/ws/session"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "use wss") {
		t.Fatalf("expected insecure remote rejection, got %v", err)
	}

	cfg.Remote = "ws://127.0.0.1:8080/ws/session"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("loopback ws should be valid: %v", err)
	}
}

func TestSaveLoadAndUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "molex.json")
	cfg := Default()
	cfg.Secret = strings.Repeat("k", 32)
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Secret != cfg.Secret || loaded.Remote != cfg.Remote {
		t.Fatalf("loaded config differs: %#v", loaded)
	}
	cfg.Tunnel.Remote = "updated-channel"
	if err := Save(path, cfg); err != nil {
		t.Fatalf("replace existing config: %v", err)
	}
	loaded, err = Load(path)
	if err != nil || loaded.Tunnel.Remote != "updated-channel" {
		t.Fatalf("updated config was not persisted: %#v, %v", loaded, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("config permissions are too broad: %v", info.Mode().Perm())
	}

	badPath := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(badPath, []byte(`{"mode":"relay","listen":"127.0.0.1:8080","unknown":true,"tunnel":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(badPath); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

func TestGenerateSecret(t *testing.T) {
	a, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || !strings.HasPrefix(a, "mx1_") || len(a) < 40 {
		t.Fatalf("unexpected generated secrets %q and %q", a, b)
	}
}

func TestNodeNameValidation(t *testing.T) {
	cfg := Default()
	cfg.Secret = strings.Repeat("s", 32)
	cfg.Tunnel.Name = "上海-Edge-01"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid node name rejected: %v", err)
	}
	cfg.Tunnel.Name = strings.Repeat("x", 65)
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "tunnel.name") {
		t.Fatalf("oversized node name accepted: %v", err)
	}
	cfg.Tunnel.Name = "bad\nname"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "control") {
		t.Fatalf("control character in node name accepted: %v", err)
	}
}
