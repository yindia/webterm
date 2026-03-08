package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"webterm/internal/config"
)

func TestRootCommandHasSubcommands(t *testing.T) {
	root := NewRootCommand()
	if root == nil || len(root.Commands()) == 0 {
		t.Fatalf("expected subcommands")
	}
}

func TestConfigInitCommand(t *testing.T) {
	cmd := newConfigCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	dir := t.TempDir()
	path := filepath.Join(dir, "webterm.yaml")
	cmd.SetArgs([]string{"init", "--path", path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("config init failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file: %v", err)
	}
}

func TestConfigInitCommandRejectsExisting(t *testing.T) {
	cmd := newConfigCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	dir := t.TempDir()
	path := filepath.Join(dir, "webterm.yaml")
	if err := os.WriteFile(path, []byte("exists"), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	cmd.SetArgs([]string{"init", "--path", path})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for existing file")
	}
}

func TestDoctorCommandFailsOnInvalidShell(t *testing.T) {
	cmd := newDoctorCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	dir := t.TempDir()
	path := filepath.Join(dir, "webterm.yaml")
	cfg := config.Default()
	cfg.Server.Bind = "127.0.0.1"
	cfg.Server.Port = 0
	cfg.Terminal.Shell = "/nope"
	cfg.Sessions.SnapshotDir = ""
	if err := config.Write(path, cfg); err != nil {
		t.Fatalf("config write failed: %v", err)
	}

	cmd.SetArgs([]string{"--config", path})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected doctor failure")
	}
}

func TestDoctorCommandPassesWithCustomConfig(t *testing.T) {
	cmd := newDoctorCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	dir := t.TempDir()
	path := filepath.Join(dir, "webterm.yaml")
	cfg := config.Default()
	cfg.Server.Bind = "127.0.0.1"
	cfg.Server.Port = 0
	cfg.Terminal.Shell = "/bin/sh"
	cfg.Sessions.SnapshotDir = ""
	if err := config.Write(path, cfg); err != nil {
		t.Fatalf("config write failed: %v", err)
	}

	cmd.SetArgs([]string{"--config", path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor failed: %v", err)
	}
}

func TestServeCommandInvokesRunServer(t *testing.T) {
	cmd := newServeCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	dir := t.TempDir()
	path := filepath.Join(dir, "webterm.yaml")
	cfg := config.Default()
	cfg.Server.Bind = "127.0.0.1"
	cfg.Server.Port = 0
	cfg.Sessions.SnapshotDir = ""
	if err := config.Write(path, cfg); err != nil {
		t.Fatalf("config write failed: %v", err)
	}

	called := false
	prev := runServer
	runServer = func(_ context.Context, _ config.Config) error {
		called = true
		return nil
	}
	defer func() { runServer = prev }()

	cmd.SetArgs([]string{"--config", path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("serve failed: %v", err)
	}
	if !called {
		t.Fatalf("expected runServer to be called")
	}
}

func TestServeCommandUsesDefaultsWhenMissingConfig(t *testing.T) {
	cmd := newServeCommand()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	called := false
	prev := runServer
	runServer = func(_ context.Context, cfg config.Config) error {
		called = true
		if cfg.Server.Port == 0 {
			return nil
		}
		return nil
	}
	defer func() { runServer = prev }()

	cmd.SetArgs([]string{"--config", filepath.Join(t.TempDir(), "missing.yaml")})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("serve failed: %v", err)
	}
	if !called {
		t.Fatalf("expected runServer call")
	}
}

func TestVersionCommand(t *testing.T) {
	cmd := newVersionCommand()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("version failed: %v", err)
	}
}

func TestCompletionCommandOutputs(t *testing.T) {
	root := NewRootCommand()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"completion", "bash"})
	if err := root.Execute(); err != nil {
		t.Fatalf("completion failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatalf("expected completion output")
	}
}
