package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateRelayConfig(t *testing.T) {
	cfg := Config{
		Mode:   ModeRelay,
		Listen: "127.0.0.1:8080",
		Tokens: []TokenEntry{
			{ID: "tok-1", Token: "mx2_0123456789abcdef", Note: "office"},
			{ID: "tok-2", Token: "mx2_fedcba9876543210", Disabled: true},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid relay config rejected: %v", err)
	}
}

func TestValidateRelayRejectsDuplicateAndShortTokens(t *testing.T) {
	cfg := Config{
		Mode:   ModeRelay,
		Listen: "127.0.0.1:8080",
		Tokens: []TokenEntry{
			{ID: "tok-1", Token: "mx2_0123456789abcdef"},
			{ID: "tok-1", Token: "mx2_0123456789abcdef"},
			{ID: "tok-3", Token: "short"},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("duplicate and short tokens were accepted")
	}
	message := err.Error()
	for _, expected := range []string{"duplicate token id", "duplicate token value", "at least 16 characters"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("validation error %q does not mention %q", message, expected)
		}
	}
}

func TestValidateTargetConfig(t *testing.T) {
	cfg := Config{
		Mode:   ModeTarget,
		Remote: "wss://relay.example.com/ws/session",
		Token:  "mx2_0123456789abcdef",
		Name:   "lab-target",
		Services: []ServiceEntry{
			{ID: "svc-1", Name: "web", Address: "10.188.200.16:30927"},
			{ID: "svc-2", Name: "ssh", Address: "127.0.0.1:22"},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid target config rejected: %v", err)
	}
}

func TestValidateTargetRejectsBadServices(t *testing.T) {
	cfg := Config{
		Mode:   ModeTarget,
		Remote: "wss://relay.example.com/ws/session",
		Token:  "mx2_0123456789abcdef",
		Services: []ServiceEntry{
			{ID: "svc-1", Name: "web", Address: "10.0.0.5:30927"},
			{ID: "svc-1", Name: "web", Address: "missing-port"},
			{ID: "", Name: "", Address: "10.0.0.5:0"},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("invalid services were accepted")
	}
	message := err.Error()
	for _, expected := range []string{"duplicate service id", "duplicate service name", "host:port", "id is required", "name is required", "between 1 and 65535"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("validation error %q does not mention %q", message, expected)
		}
	}
}

func TestValidateEdgeConfig(t *testing.T) {
	cfg := Config{
		Mode:   ModeEdge,
		Remote: "wss://relay.example.com",
		Token:  "mx2_0123456789abcdef",
		Mappings: []MappingEntry{
			{Service: "svc-1", Port: 28080},
			{Service: "svc-2", Port: 28090, LAN: true},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid edge config rejected: %v", err)
	}
}

func TestValidateEdgeRejectsDuplicateMappings(t *testing.T) {
	cfg := Config{
		Mode:   ModeEdge,
		Remote: "wss://relay.example.com",
		Token:  "mx2_0123456789abcdef",
		Mappings: []MappingEntry{
			{Service: "svc-1", Port: 28080},
			{Service: "svc-1", Port: 28080},
			{Service: "svc-2", Port: 0},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("invalid mappings were accepted")
	}
	message := err.Error()
	for _, expected := range []string{"duplicate mapping", "duplicate local port", "between 1 and 65535"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("validation error %q does not mention %q", message, expected)
		}
	}
}

func TestValidateClientRequiresTokenAndSecureRemote(t *testing.T) {
	cfg := Config{Mode: ModeEdge, Remote: "ws://relay.example.com/ws/session", Token: "short"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("insecure remote and short token were accepted")
	}
	message := err.Error()
	if !strings.Contains(message, "loopback") || !strings.Contains(message, "at least 16 characters") {
		t.Fatalf("validation error %q lacks remote and token guidance", message)
	}
}

func TestMappingListenAddress(t *testing.T) {
	local := MappingEntry{Service: "svc", Port: 28080}
	if address := local.ListenAddress(); address != "127.0.0.1:28080" {
		t.Fatalf("loopback mapping address = %q", address)
	}
	lan := MappingEntry{Service: "svc", Port: 28080, LAN: true}
	if address := lan.ListenAddress(); address != "0.0.0.0:28080" {
		t.Fatalf("LAN mapping address = %q", address)
	}
}

func TestLoadRejectsLegacyPunchConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "molex.json")
	legacy := `{
  "mode": "punch",
  "role": "edge",
  "secret": "mx1_legacy-secret-value",
  "token": "legacy-token-0123456789",
  "listen": "127.0.0.1:2222",
  "remote": "wss://relay.example.com/ws/session",
  "tunnel": {"local": "127.0.0.1:22", "remote": "home-ssh"}
}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if !errors.Is(err, ErrLegacyConfig) {
		t.Fatalf("legacy config error = %v, want ErrLegacyConfig", err)
	}
	if !strings.Contains(err.Error(), "molex config init") {
		t.Fatalf("legacy config error %q lacks migration guidance", err)
	}
}

func TestLoadRejectsLegacyFieldsEvenWithNewMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "molex.json")
	mixed := `{"mode": "edge", "secret": "mx1_leftover-secret"}`
	if err := os.WriteFile(path, []byte(mixed), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); !errors.Is(err, ErrLegacyConfig) {
		t.Fatalf("mixed legacy config error = %v, want ErrLegacyConfig", err)
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "molex.json")
	if err := os.WriteFile(path, []byte(`{"mode":"edge","surprise":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v", err)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "molex.json")
	cfg := Config{
		Mode:   ModeTarget,
		Remote: "relay.example.com",
		Token:  "  mx2_0123456789abcdef  ",
		Services: []ServiceEntry{
			{ID: "svc-1", Name: " web ", Address: " 10.0.0.5:30927 "},
		},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.Remote != "wss://relay.example.com/ws/session" {
		t.Fatalf("remote = %q, want normalized wss URL with default path", loaded.Remote)
	}
	if loaded.Token != "mx2_0123456789abcdef" {
		t.Fatalf("token = %q, want trimmed", loaded.Token)
	}
	if loaded.Services[0].Name != "web" || loaded.Services[0].Address != "10.0.0.5:30927" {
		t.Fatalf("service = %#v, want trimmed fields", loaded.Services[0])
	}
}

func TestSaveRejectsInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "molex.json")
	if err := Save(path, Config{Mode: "punch"}); err == nil {
		t.Fatal("invalid mode was saved")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid config left a file behind")
	}
}

func TestGenerateTokenAndID(t *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, "mx2_") || len(token) < MinTokenLength {
		t.Fatalf("token = %q, want mx2_ prefix and sufficient length", token)
	}
	id, err := GenerateID("svc")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "svc-") || len(id) != len("svc-")+10 {
		t.Fatalf("id = %q, want svc-<10 hex chars>", id)
	}
	other, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	if token == other {
		t.Fatal("two generated tokens are identical")
	}
}

func TestFindTokenAndService(t *testing.T) {
	cfg := Config{
		Tokens:   []TokenEntry{{ID: "tok-1", Token: "mx2_0123456789abcdef"}},
		Services: []ServiceEntry{{ID: "svc-1", Name: "web", Address: "10.0.0.5:80"}},
	}
	if _, ok := cfg.FindToken("mx2_0123456789abcdef"); !ok {
		t.Fatal("existing token not found")
	}
	if _, ok := cfg.FindToken("missing"); ok {
		t.Fatal("missing token reported as found")
	}
	if _, ok := cfg.FindService("svc-1"); !ok {
		t.Fatal("existing service not found")
	}
	if _, ok := cfg.FindService("svc-2"); ok {
		t.Fatal("missing service reported as found")
	}
}

func TestDefaultIsEdge(t *testing.T) {
	cfg := Default()
	if cfg.Mode != ModeEdge {
		t.Fatalf("default mode = %q, want edge", cfg.Mode)
	}
}

func TestValidateMultiGroupTargetWithServiceVisibility(t *testing.T) {
	cfg := Config{
		Mode:   ModeTarget,
		Remote: "wss://relay.example.com/ws/session",
		Tokens: []TokenEntry{
			{ID: "office", Token: "mx2_office-token-0123456789"},
			{ID: "family", Token: "mx2_family-token-9876543210"},
		},
		Services: []ServiceEntry{
			{ID: "svc-1", Name: "everyone", Address: "10.0.0.5:80"},
			{ID: "svc-2", Name: "office-only", Address: "10.0.0.5:22", Groups: []string{"office"}},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("multi-group target rejected: %v", err)
	}
	if !cfg.Services[0].VisibleTo("family") || !cfg.Services[0].VisibleTo("office") {
		t.Fatal("unrestricted service must be visible to every group")
	}
	if cfg.Services[1].VisibleTo("family") || !cfg.Services[1].VisibleTo("office") {
		t.Fatal("restricted service visibility is wrong")
	}
}

func TestValidateMultiGroupRules(t *testing.T) {
	cfg := Config{
		Mode:   ModeTarget,
		Remote: "wss://relay.example.com/ws/session",
		Tokens: []TokenEntry{
			{ID: "", Token: "mx2_office-token-0123456789"},
			{ID: "office", Token: "mx2_office-token-0123456789"},
		},
		Services: []ServiceEntry{
			{ID: "svc-1", Name: "web", Address: "10.0.0.5:80", Groups: []string{"missing"}},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("invalid multi-group config accepted")
	}
	message := err.Error()
	for _, expected := range []string{"group name is required when joining several groups", "duplicate token value", `unknown group name "missing"`} {
		if !strings.Contains(message, expected) {
			t.Fatalf("validation error %q does not mention %q", message, expected)
		}
	}

	both := Config{
		Mode:   ModeEdge,
		Remote: "wss://relay.example.com/ws/session",
		Token:  "mx2_single-token-0123456789",
		Tokens: []TokenEntry{{ID: "office", Token: "mx2_office-token-0123456789"}},
	}
	if err := both.Validate(); err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("mixing token and tokens should fail, got %v", err)
	}
}

func TestValidateEdgeMappingGroups(t *testing.T) {
	cfg := Config{
		Mode:   ModeEdge,
		Remote: "wss://relay.example.com/ws/session",
		Tokens: []TokenEntry{
			{ID: "office", Token: "mx2_office-token-0123456789"},
			{ID: "family", Token: "mx2_family-token-9876543210"},
		},
		Mappings: []MappingEntry{
			{Service: "svc-1", Group: "office", Port: 28080},
			{Service: "svc-1", Group: "family", Port: 28081},
			{Service: "svc-2", Port: 28082},
			{Service: "svc-3", Group: "missing", Port: 28083},
		},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("invalid mapping groups accepted")
	}
	message := err.Error()
	if !strings.Contains(message, "group is required when the edge joined several groups") {
		t.Fatalf("missing-group error absent: %q", message)
	}
	if !strings.Contains(message, `unknown group name "missing"`) {
		t.Fatalf("unknown-group error absent: %q", message)
	}

	// The same service mapped once per group is allowed.
	valid := cfg
	valid.Mappings = cfg.Mappings[:2]
	if err := valid.Validate(); err != nil {
		t.Fatalf("per-group duplicate service rejected: %v", err)
	}
}

func TestValidateRelayRotationFields(t *testing.T) {
	base := TokenEntry{ID: "tok-1", Token: "mx2_current-token-0123456789"}
	cfg := Config{Mode: ModeRelay, Listen: "127.0.0.1:8080"}

	rotated := base
	rotated.PreviousToken = "mx2_previous-token-9876543210"
	rotated.PreviousExpiresAt = time.Now().Add(72 * time.Hour)
	cfg.Tokens = []TokenEntry{rotated}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid rotation state rejected: %v", err)
	}

	missingExpiry := base
	missingExpiry.PreviousToken = "mx2_previous-token-9876543210"
	cfg.Tokens = []TokenEntry{missingExpiry}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "previousExpiresAt is required") {
		t.Fatalf("missing expiry accepted: %v", err)
	}

	samePrevious := base
	samePrevious.PreviousToken = base.Token
	samePrevious.PreviousExpiresAt = time.Now().Add(time.Hour)
	cfg.Tokens = []TokenEntry{samePrevious}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "must differ") {
		t.Fatalf("identical previous token accepted: %v", err)
	}
}

func TestParseLifetimePresets(t *testing.T) {
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	never, err := ParseLifetime(LifetimeNever, now)
	if err != nil || !never.IsZero() {
		t.Fatalf("never = %v %v", never, err)
	}
	empty, err := ParseLifetime("", now)
	if err != nil || !empty.IsZero() {
		t.Fatalf("empty = %v %v", empty, err)
	}
	month, err := ParseLifetime(Lifetime30Days, now)
	if err != nil || !month.Equal(now.Add(30*24*time.Hour)) {
		t.Fatalf("30d = %v %v", month, err)
	}
	if _, err := ParseLifetime("2h", now); err == nil || !strings.Contains(err.Error(), "lifetime must be one of") {
		t.Fatalf("unknown lifetime accepted: %v", err)
	}
}

func TestGroupTokensFallsBackToSingleToken(t *testing.T) {
	single := Config{Mode: ModeEdge, Token: "mx2_single-token-0123456789"}
	groups := single.GroupTokens()
	if len(groups) != 1 || groups[0].ID != "" || groups[0].Token != single.Token {
		t.Fatalf("single-token groups = %#v", groups)
	}
	multi := Config{Mode: ModeTarget, Tokens: []TokenEntry{{ID: "a", Token: "mx2_a-token-0123456789abc"}}}
	if got := multi.GroupTokens(); len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("multi-token groups = %#v", got)
	}
}
