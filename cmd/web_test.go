package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWebPasswordFromEnvironment(t *testing.T) {
	t.Setenv(webPasswordEnvironment, "environment-password\n")
	password, err := loadWebPassword("")
	if err != nil {
		t.Fatal(err)
	}
	if password != "environment-password" {
		t.Fatalf("unexpected password %q", password)
	}
}

func TestLoadWebPasswordFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "password")
	if err := os.WriteFile(path, []byte("file-password\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	password, err := loadWebPassword(path)
	if err != nil {
		t.Fatal(err)
	}
	if password != "file-password" {
		t.Fatalf("unexpected password %q", password)
	}
}

func TestLoadWebPasswordRequiresCredential(t *testing.T) {
	t.Setenv(webPasswordEnvironment, "")
	if _, err := loadWebPassword(""); err == nil {
		t.Fatal("expected missing password error")
	}
}
