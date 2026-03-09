package terminal

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/GianlucaP106/gotmux/gotmux"
	"webterm/internal/config"
)

func TestCreateWriteReadClose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty test is unix-first in current implementation")
	}
	if _, err := gotmux.DefaultTmux(); err == nil {
		t.Skip("tmux present; test assumes pty backend")
	}

	cfg := config.Default()
	cfg.Sessions.SnapshotDir = t.TempDir()
	cfg.Sessions.MaxSessions = 2
	cfg.Terminal.Shell = "/bin/bash"

	mgr, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	t.Cleanup(mgr.CloseAll)

	s, err := mgr.Create("test", 120, 30)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if _, err := mgr.Write(s.ID, []byte("echo webterm_smoke\n")); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	buf := make([]byte, 4096)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		n, readErr := mgr.Read(s.ID, buf)
		if n > 0 && strings.Contains(string(buf[:n]), "webterm_smoke") {
			if closeErr := mgr.Close(s.ID); closeErr != nil {
				t.Fatalf("Close failed: %v", closeErr)
			}
			return
		}
		if readErr != nil {
			t.Fatalf("Read failed: %v", readErr)
		}
	}

	t.Fatalf("did not observe expected output before timeout")
}
