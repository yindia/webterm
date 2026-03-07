package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfigHasExpectedValues(t *testing.T) {
	cfg := Default()
	if cfg.Server.Port == 0 {
		t.Fatalf("expected default server port to be set")
	}
	if cfg.Auth.Mode == "" {
		t.Fatalf("expected auth mode default")
	}
	if cfg.Sessions.MaxSessions == 0 {
		t.Fatalf("expected max sessions default")
	}
	if cfg.Sessions.SnapshotInterval <= 0 {
		t.Fatalf("expected snapshot interval default")
	}
}

func TestWriteLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "webterm.yaml")

	cfg := Default()
	cfg.Server.Port = 9090
	cfg.Auth.Mode = "token"
	cfg.Auth.Token = "abc123"
	cfg.Sessions.SnapshotDir = ""
	cfg.Sessions.SnapshotInterval = 12 * time.Second

	if err := Write(path, cfg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Server.Port != 9090 {
		t.Fatalf("expected port to round-trip")
	}
	if loaded.Auth.Mode != "token" {
		t.Fatalf("expected auth mode to round-trip")
	}
	if loaded.Auth.Token != "abc123" {
		t.Fatalf("expected token to round-trip")
	}
	if loaded.Sessions.SnapshotInterval != 12*time.Second {
		t.Fatalf("expected snapshot interval to round-trip")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatalf("expected error for missing file")
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("server: ["), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatalf("expected yaml error")
	}
}

func TestWriteCreatesDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "webterm.yaml")
	if err := Write(path, Default()); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestWriteFailsWhenParentIsFile(t *testing.T) {
	dir := t.TempDir()
	parent := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	path := filepath.Join(parent, "child.yaml")
	if err := Write(path, Default()); err == nil {
		t.Fatalf("expected error for invalid parent")
	}
}
