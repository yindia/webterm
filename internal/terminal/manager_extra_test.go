package terminal

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/creack/pty"

	"webterm/internal/config"
)

func TestSnapshotEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	plain := []byte("hello snapshot")
	ct, nonce, err := encryptSnapshot(plain, key)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	out, err := decryptSnapshot(ct, nonce, key)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if string(out) != string(plain) {
		t.Fatalf("expected round-trip")
	}
}

func TestResolveSnapshotDir(t *testing.T) {
	if dir, err := resolveSnapshotDir(""); err != nil || dir != "" {
		t.Fatalf("expected empty dir")
	}
	if dir, err := resolveSnapshotDir("~/snapshots"); err != nil || dir == "" {
		t.Fatalf("expected expanded dir")
	}
}

func TestResolveCredential(t *testing.T) {
	if cred, err := resolveCredential("", ""); err != nil || cred != nil {
		t.Fatalf("expected nil credential")
	}

	current, err := user.Current()
	if err != nil {
		t.Skip("cannot resolve current user")
	}
	cred, err := resolveCredential(current.Username, "")
	if err != nil {
		t.Fatalf("resolveCredential failed: %v", err)
	}
	if cred == nil {
		t.Fatalf("expected credential")
	}
}

func TestHistoryReturnsCopy(t *testing.T) {
	mgr := &Manager{sessions: map[string]*Session{}}
	s := &Session{ID: "s1", Name: "t", CreatedAt: time.Now(), LastActive: time.Now()}
	s.buffer = []byte("abc")
	mgr.sessions[s.ID] = s

	buf, err := mgr.History(s.ID)
	if err != nil {
		t.Fatalf("History failed: %v", err)
	}
	if string(buf) != "abc" {
		t.Fatalf("expected buffer content")
	}
	buf[0] = 'z'
	if string(s.buffer) == string(buf) {
		t.Fatalf("expected copy")
	}
}

func TestHistoryMissingSession(t *testing.T) {
	mgr := &Manager{sessions: map[string]*Session{}}
	if _, err := mgr.History("missing"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestAppendOutputTriggersSnapshot(t *testing.T) {
	mgr := &Manager{
		snapshotDir:      t.TempDir(),
		snapshotInterval: 0,
		snapshotCh:       make(chan snapshotRecord, 1),
	}
	s := &Session{ID: "s1", Name: "t", LastCols: 80, LastRows: 24}
	data := []byte("hello")
	mgr.appendOutput(s, data)

	select {
	case rec := <-mgr.snapshotCh:
		if rec.ID != "s1" {
			t.Fatalf("expected snapshot id")
		}
	default:
		t.Fatalf("expected snapshot record")
	}
}

func TestAppendOutputEncryptedSnapshot(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	mgr := &Manager{
		snapshotDir:      t.TempDir(),
		snapshotInterval: 0,
		snapshotCh:       make(chan snapshotRecord, 1),
		snapshotKey:      key,
	}
	s := &Session{ID: "s2", Name: "t", LastCols: 80, LastRows: 24}
	mgr.appendOutput(s, []byte("secret"))

	select {
	case rec := <-mgr.snapshotCh:
		if !rec.Encrypted || rec.Nonce == "" {
			t.Fatalf("expected encrypted snapshot")
		}
	default:
		t.Fatalf("expected snapshot record")
	}
}

func TestAppendOutputTrimsBuffer(t *testing.T) {
	mgr := &Manager{}
	s := &Session{ID: "s1"}
	data := make([]byte, maxSessionBuffer+10)
	for i := range data {
		data[i] = 'a'
	}
	mgr.appendOutput(s, data)
	if len(s.buffer) != maxSessionBuffer {
		t.Fatalf("expected buffer trimmed")
	}
}

func TestEncryptSnapshotInvalidKey(t *testing.T) {
	if _, _, err := encryptSnapshot([]byte("x"), []byte("short")); err == nil {
		t.Fatalf("expected error for invalid key")
	}
}

func TestStartShellInvalidPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty test is unix-first in current implementation")
	}
	if _, _, err := startShell("/nope", "/", nil, 80, 24, "", "", false); err == nil {
		t.Fatalf("expected error")
	}
}

func TestCloseRemovesSnapshot(t *testing.T) {
	dir := t.TempDir()
	mgr := &Manager{snapshotDir: dir, sessions: map[string]*Session{}}
	s := &Session{ID: "s1"}
	mgr.sessions[s.ID] = s
	if err := os.WriteFile(mgr.snapshotPath("s1"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := mgr.Close("s1"); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if _, err := os.Stat(mgr.snapshotPath("s1")); err == nil {
		t.Fatalf("expected snapshot removed")
	}
}

func TestManagerErrorPaths(t *testing.T) {
	mgr := &Manager{sessions: map[string]*Session{}}
	if _, err := mgr.Write("missing", []byte("x")); err == nil {
		t.Fatalf("expected write error for missing session")
	}
	if _, err := mgr.Read("missing", make([]byte, 1)); err == nil {
		t.Fatalf("expected read error for missing session")
	}
	if err := mgr.Resize("missing", 10, 10); err == nil {
		t.Fatalf("expected resize error")
	}
	if err := mgr.Close("missing"); err == nil {
		t.Fatalf("expected close error")
	}
}

func TestNewAcceptsSnapshotKeyString(t *testing.T) {
	cfg := config.Default()
	cfg.Sessions.SnapshotKey = "not-base64"
	if _, err := New(cfg); err != nil {
		t.Fatalf("expected snapshot key to be accepted: %v", err)
	}
}

func TestNewDefaultsApply(t *testing.T) {
	cfg := config.Default()
	cfg.Sessions.MaxSessions = 0
	cfg.Sessions.IdleTimeout = 0
	cfg.Sessions.SnapshotInterval = 0
	cfg.Sessions.SnapshotDir = ""
	mgr, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if mgr.maxSessions == 0 {
		t.Fatalf("expected defaults applied")
	}
}

func TestWriteSnapshotCreatesFile(t *testing.T) {
	dir := t.TempDir()
	mgr := &Manager{snapshotDir: dir}
	rec := snapshotRecord{ID: "s1", Name: "t", UpdatedAt: time.Now(), Buffer: ""}
	mgr.writeSnapshot(rec)
	if _, err := os.Stat(mgr.snapshotPath("s1")); err != nil {
		t.Fatalf("expected snapshot file: %v", err)
	}
}

func TestRestoreSnapshotsSkipsInvalid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "empty.json"), []byte(`{"id":""}`), 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	mgr := &Manager{snapshotDir: dir, maxSessions: 1, sessions: map[string]*Session{}}
	mgr.restoreSnapshots()
	if len(mgr.sessions) != 0 {
		t.Fatalf("expected no sessions restored")
	}
}

func TestResolveWorkingDir(t *testing.T) {
	if dir, err := resolveWorkingDir("~"); err != nil || dir == "" {
		t.Fatalf("expected home dir")
	}
	if dir, err := resolveWorkingDir("~/tmp"); err != nil || dir == "" {
		t.Fatalf("expected expanded dir")
	}
	if dir, err := resolveWorkingDir(""); err != nil || dir == "" {
		t.Fatalf("expected home dir for empty input")
	}
}

func TestMaxSessionsLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty test is unix-first in current implementation")
	}
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("shell not available")
	}
	cfg := config.Default()
	cfg.Sessions.MaxSessions = 1
	cfg.Terminal.Shell = "/bin/bash"
	cfg.Sessions.SnapshotDir = ""
	mgr, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer mgr.CloseAll()

	if _, err := mgr.Create("one", 80, 24); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if _, err := mgr.Create("two", 80, 24); err == nil {
		t.Fatalf("expected max session error")
	}
}

func TestCloseAllClearsSessions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty test is unix-first in current implementation")
	}
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("shell not available")
	}
	cfg := config.Default()
	cfg.Sessions.MaxSessions = 2
	cfg.Terminal.Shell = "/bin/bash"
	cfg.Sessions.SnapshotDir = ""
	mgr, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if _, err := mgr.Create("one", 80, 24); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if _, err := mgr.Create("two", 80, 24); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	mgr.CloseAll()
	if len(mgr.List()) != 0 {
		t.Fatalf("expected sessions cleared")
	}
}

func TestRestoreSnapshotsCreatesSession(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty test is unix-first in current implementation")
	}
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("shell not available")
	}

	dir := t.TempDir()
	rec := snapshotRecord{
		ID:        "snap1",
		Name:      "snap",
		Cols:      80,
		Rows:      24,
		UpdatedAt: time.Now(),
		Buffer:    base64.StdEncoding.EncodeToString([]byte("snapshot-data")),
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "snap1.json"), raw, 0o600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	cfg := config.Default()
	cfg.Sessions.SnapshotDir = dir
	cfg.Sessions.SnapshotInterval = time.Second
	cfg.Terminal.Shell = "/bin/bash"
	mgr, err := New(cfg)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer mgr.CloseAll()

	if len(mgr.List()) != 1 {
		t.Fatalf("expected restored session")
	}
}

func TestDecryptSnapshotInvalidNonce(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	if _, err := decryptSnapshot([]byte("abc"), []byte("short"), key); err == nil {
		t.Fatalf("expected nonce error")
	}
}

func TestResolveSnapshotDirHome(t *testing.T) {
	if dir, err := resolveSnapshotDir("~"); err != nil || dir == "" {
		t.Fatalf("expected home snapshot dir")
	}
}

func TestResolveCredentialInvalidUser(t *testing.T) {
	if _, err := resolveCredential("this-user-should-not-exist", ""); err == nil {
		t.Fatalf("expected error for invalid user")
	}
}

func TestResolveCredentialNumericGroup(t *testing.T) {
	cred, err := resolveCredential("", "0")
	if err != nil {
		t.Fatalf("expected numeric group to resolve: %v", err)
	}
	if cred == nil {
		t.Fatalf("expected credential")
	}
}

func TestRandomIDLength(t *testing.T) {
	val, err := randomID(8)
	if err != nil {
		t.Fatalf("randomID failed: %v", err)
	}
	if len(val) != 16 {
		t.Fatalf("expected hex length 16")
	}
}

func TestStartWithFallbackShells(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty test is unix-first in current implementation")
	}
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("shell not available")
	}
	ptmx, cmd, err := startWithFallbackShells("/bin/does-not-exist", "/", nil, 80, 24, "", "")
	if err != nil {
		t.Fatalf("expected fallback shell: %v", err)
	}
	_ = ptmx.Close()
	_ = cmd.Process.Kill()
}

func TestStartShellSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty test is unix-first in current implementation")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("shell not available")
	}
	ptmx, cmd, err := startShell("/bin/sh", "/", nil, 80, 24, "", "", false)
	if err != nil {
		t.Fatalf("expected success: %v", err)
	}
	_ = ptmx.Close()
	_ = cmd.Process.Kill()
}

func TestResizeSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty test is unix-first in current implementation")
	}
	ptmx, _, err := pty.Open()
	if err != nil {
		t.Skip("pty open failed")
	}
	defer func() { _ = ptmx.Close() }()

	s := &Session{ID: "s1", ptmx: ptmx}
	mgr := &Manager{sessions: map[string]*Session{"s1": s}}
	if err := mgr.Resize("s1", 90, 30); err != nil {
		t.Fatalf("resize failed: %v", err)
	}
}

func TestSnapshotWorkerWritesFile(t *testing.T) {
	dir := t.TempDir()
	mgr := &Manager{snapshotDir: dir, snapshotCh: make(chan snapshotRecord, 1)}
	go func() {
		mgr.snapshotWorker()
	}()
	rec := snapshotRecord{ID: "s1", Name: "t", UpdatedAt: time.Now()}
	mgr.snapshotCh <- rec
	close(mgr.snapshotCh)
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(mgr.snapshotPath("s1")); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := os.Stat(mgr.snapshotPath("s1")); err != nil {
		t.Fatalf("expected snapshot file: %v", err)
	}
}

func TestSweepIdleSessions(t *testing.T) {
	mgr := &Manager{idleTimeout: time.Millisecond, sessions: map[string]*Session{}}
	mgr.sessions["s1"] = &Session{ID: "s1", LastActive: time.Now().Add(-time.Hour)}

	prev := idleSweepInterval
	idleSweepInterval = 5 * time.Millisecond
	defer func() { idleSweepInterval = prev }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go mgr.sweepIdleSessions(ctx)

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(mgr.List()) == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(mgr.List()) != 0 {
		t.Fatalf("expected idle session swept")
	}
}
