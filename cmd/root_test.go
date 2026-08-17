package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/suifei/molex/internal/config"
)

func TestVersionCommand(t *testing.T) {
	root := newRootCommand("1.2.3-test")
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"version"})
	root.SetContext(context.Background())
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) != "1.2.3-test" {
		t.Fatalf("unexpected output %q", output.String())
	}
}

func TestRootExposesWebWithoutDesktopGUI(t *testing.T) {
	root := newRootCommand("test")
	commands := make(map[string]bool)
	for _, command := range root.Commands() {
		commands[command.Name()] = true
	}
	if !commands["web"] {
		t.Fatal("web command is not registered")
	}
	if commands["gui"] {
		t.Fatal("desktop gui command must not be registered")
	}
}

func TestWebCommandDefaultsToAutomaticLoopbackManagement(t *testing.T) {
	command := newWebCommand()
	listen := command.Flags().Lookup("listen")
	openBrowser := command.Flags().Lookup("open-browser")
	if listen == nil || listen.DefValue != "127.0.0.1:9090" {
		t.Fatalf("default WebUI listen = %#v, want 127.0.0.1:9090", listen)
	}
	if openBrowser == nil || openBrowser.DefValue != "true" {
		t.Fatalf("default open-browser = %#v, want true", openBrowser)
	}
}

func TestConnectCommandUsesV2Flags(t *testing.T) {
	flags := newConnectCommand().Flags()
	for _, name := range []string{"config", "remote", "token", "name"} {
		if flags.Lookup(name) == nil {
			t.Fatalf("connect flag %q is missing", name)
		}
	}
	for _, name := range []string{"secret", "channel", "role", "pool", "local", "listen"} {
		if flags.Lookup(name) != nil {
			t.Fatalf("legacy v1 connect flag %q must be removed", name)
		}
	}
}

func TestConfigInitAndCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "molex.json")
	root := newRootCommand("test")
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"config", "init", "--config", path, "--mode", "target"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = newRootCommand("test")
	output := new(bytes.Buffer)
	root.SetOut(output)
	root.SetArgs([]string{"config", "check", "--config", path})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "valid") {
		t.Fatalf("unexpected check output %q", output.String())
	}
}

func TestConfigInitRelayGeneratesToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.json")
	root := newRootCommand("test")
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"config", "init", "--config", path, "--mode", "relay"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != config.ModeRelay || len(cfg.Tokens) != 1 {
		t.Fatalf("relay init config = %#v", cfg)
	}
	if !strings.HasPrefix(cfg.Tokens[0].Token, "mx2_") || cfg.Tokens[0].ID == "" {
		t.Fatalf("relay init token = %#v", cfg.Tokens[0])
	}
}

func TestConfigCheckRejectsLegacyLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "molex.json")
	legacy := `{"mode":"punch","role":"edge","secret":"mx1_old-secret-value","remote":"wss://relay.example/ws/session","tunnel":{"local":"127.0.0.1:22","remote":"ssh"}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	root := newRootCommand("test")
	root.SetOut(new(bytes.Buffer))
	root.SetErr(new(bytes.Buffer))
	root.SetArgs([]string{"config", "check", "--config", path})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "molex config init") {
		t.Fatalf("legacy check error = %v, want migration guidance", err)
	}
}
