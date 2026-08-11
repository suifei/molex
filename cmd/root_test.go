package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
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

func TestConfigInitAndCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "molex.json")
	root := newRootCommand("test")
	root.SetOut(new(bytes.Buffer))
	root.SetArgs([]string{"config", "init", "--config", path, "--mode", "punch", "--role", "target"})
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
