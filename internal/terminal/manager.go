package terminal

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"

	"webterm/internal/config"
)

type Session struct {
	ID           string
	Name         string
	CreatedAt    time.Time
	LastActive   time.Time
	LastCols     uint16
	LastRows     uint16
	LastSnapshot time.Time

	ptmx   *os.File
	cmd    *exec.Cmd
	closed bool
	buffer []byte
	mu     sync.Mutex
}

const maxSessionBuffer = 2 * 1024 * 1024

var idleSweepInterval = 30 * time.Second

type snapshotRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Cols      uint16    `json:"cols"`
	Rows      uint16    `json:"rows"`
	UpdatedAt time.Time `json:"updated_at"`
	Buffer    string    `json:"buffer"`
	Nonce     string    `json:"nonce"`
	Encrypted bool      `json:"encrypted"`
}

type Manager struct {
	maxSessions      int
	idleTimeout      time.Duration
	shell            string
	workingDir       string
	env              []string
	user             string
	group            string
	snapshotDir      string
	snapshotInterval time.Duration
	snapshotCh       chan snapshotRecord
	snapshotKey      []byte

	mu       sync.RWMutex
	sessions map[string]*Session
}

func New(cfg config.Config) (*Manager, error) {
	workingDir, err := resolveWorkingDir(cfg.Terminal.WorkingDir)
	if err != nil {
		return nil, err
	}
	snapshotDir, err := resolveSnapshotDir(cfg.Sessions.SnapshotDir)
	if err != nil {
		return nil, err
	}

	if cfg.Sessions.MaxSessions <= 0 {
		cfg.Sessions.MaxSessions = 10
	}
	if cfg.Sessions.IdleTimeout <= 0 {
		cfg.Sessions.IdleTimeout = 24 * time.Hour
	}

	m := &Manager{
		maxSessions:      cfg.Sessions.MaxSessions,
		idleTimeout:      cfg.Sessions.IdleTimeout,
		shell:            cfg.Terminal.Shell,
		workingDir:       workingDir,
		env:              cfg.Terminal.Env,
		user:             cfg.Terminal.User,
		group:            cfg.Terminal.Group,
		sessions:         map[string]*Session{},
		snapshotDir:      snapshotDir,
		snapshotInterval: cfg.Sessions.SnapshotInterval,
		snapshotKey:      nil,
	}

	if strings.TrimSpace(cfg.Sessions.SnapshotKey) != "" {
		key, err := base64.StdEncoding.DecodeString(cfg.Sessions.SnapshotKey)
		if err != nil {
			return nil, err
		}
		if len(key) != 32 {
			return nil, errors.New("snapshot_key must be 32 bytes base64")
		}
		m.snapshotKey = key
	}

	if m.snapshotInterval <= 0 {
		m.snapshotInterval = 5 * time.Second
	}
	if m.snapshotDir != "" {
		if err := os.MkdirAll(m.snapshotDir, 0o755); err != nil {
			return nil, err
		}
		m.snapshotCh = make(chan snapshotRecord, 32)
		go m.snapshotWorker()
		m.restoreSnapshots()
	}

	go m.sweepIdleSessions(context.Background())

	return m, nil
}

func (m *Manager) Create(name string, cols uint16, rows uint16) (*Session, error) {
	id, err := randomID(16)
	if err != nil {
		return nil, err
	}
	return m.createWithID(id, name, cols, rows, nil)
}

func (m *Manager) createWithID(id string, name string, cols uint16, rows uint16, buffer []byte) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.sessions) >= m.maxSessions {
		return nil, errors.New("maximum session limit reached")
	}
	if _, exists := m.sessions[id]; exists {
		return nil, errors.New("session already exists")
	}

	if name == "" {
		name = "Terminal"
	}

	ptmx, cmd, err := startWithFallbackShells(m.shell, m.workingDir, m.env, cols, rows, m.user, m.group)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	s := &Session{
		ID:           id,
		Name:         name,
		CreatedAt:    now,
		LastActive:   now,
		LastCols:     cols,
		LastRows:     rows,
		LastSnapshot: now,
		ptmx:         ptmx,
		cmd:          cmd,
	}
	if len(buffer) > 0 {
		if len(buffer) > maxSessionBuffer {
			buffer = buffer[len(buffer)-maxSessionBuffer:]
		}
		s.buffer = append([]byte{}, buffer...)
	}

	m.sessions[id] = s
	return s, nil
}

func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

func (m *Manager) List() []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	return out
}

func (m *Manager) Close(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	if !ok {
		return errors.New("session not found")
	}

	if m.snapshotDir != "" {
		_ = os.Remove(m.snapshotPath(id))
	}

	return s.close()
}

func (m *Manager) CloseAll() {
	m.mu.Lock()
	all := make([]*Session, 0, len(m.sessions))
	for id, s := range m.sessions {
		all = append(all, s)
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	for _, s := range all {
		_ = s.close()
	}
}

func (m *Manager) Write(id string, data []byte) (int, error) {
	s, ok := m.Get(id)
	if !ok {
		return 0, errors.New("session not found")
	}
	s.touch()
	return s.ptmx.Write(data)
}

func (m *Manager) Read(id string, buf []byte) (int, error) {
	s, ok := m.Get(id)
	if !ok {
		return 0, io.EOF
	}
	n, err := s.ptmx.Read(buf)
	if n > 0 {
		s.touch()
		m.appendOutput(s, buf[:n])
	}
	return n, err
}

func (m *Manager) Resize(id string, cols uint16, rows uint16) error {
	s, ok := m.Get(id)
	if !ok {
		return errors.New("session not found")
	}
	s.touch()
	s.mu.Lock()
	s.LastCols = cols
	s.LastRows = rows
	s.mu.Unlock()
	return pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows})
}

func (m *Manager) History(id string) ([]byte, error) {
	s, ok := m.Get(id)
	if !ok {
		return nil, errors.New("session not found")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.buffer) == 0 {
		return nil, nil
	}
	out := make([]byte, len(s.buffer))
	copy(out, s.buffer)
	return out, nil
}

func (s *Session) close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
		_, _ = s.cmd.Process.Wait()
	}

	if s.ptmx != nil {
		return s.ptmx.Close()
	}
	return nil
}

func (s *Session) touch() {
	s.mu.Lock()
	s.LastActive = time.Now().UTC()
	s.mu.Unlock()
}

func (m *Manager) appendOutput(s *Session, data []byte) {
	if len(data) == 0 {
		return
	}
	var snapshot snapshotRecord
	var bufCopy []byte
	var shouldSnapshot bool
	now := time.Now().UTC()

	s.mu.Lock()
	if len(data) >= maxSessionBuffer {
		s.buffer = append([]byte{}, data[len(data)-maxSessionBuffer:]...)
	} else {
		if len(s.buffer)+len(data) > maxSessionBuffer {
			drop := len(s.buffer) + len(data) - maxSessionBuffer
			s.buffer = append([]byte{}, s.buffer[drop:]...)
		}
		s.buffer = append(s.buffer, data...)
	}

	if m.snapshotCh != nil && m.snapshotDir != "" {
		if now.Sub(s.LastSnapshot) >= m.snapshotInterval {
			s.LastSnapshot = now
			bufCopy = append([]byte{}, s.buffer...)
			snapshot = snapshotRecord{
				ID:        s.ID,
				Name:      s.Name,
				Cols:      s.LastCols,
				Rows:      s.LastRows,
				UpdatedAt: now,
			}
			shouldSnapshot = true
		}
	}
	s.mu.Unlock()

	if shouldSnapshot {
		if len(m.snapshotKey) > 0 {
			ciphertext, nonce, err := encryptSnapshot(bufCopy, m.snapshotKey)
			if err == nil {
				snapshot.Buffer = base64.StdEncoding.EncodeToString(ciphertext)
				snapshot.Nonce = base64.StdEncoding.EncodeToString(nonce)
				snapshot.Encrypted = true
			} else {
				snapshot.Buffer = base64.StdEncoding.EncodeToString(bufCopy)
			}
		} else {
			snapshot.Buffer = base64.StdEncoding.EncodeToString(bufCopy)
		}
		select {
		case m.snapshotCh <- snapshot:
		default:
		}
	}
}

func (m *Manager) snapshotWorker() {
	for rec := range m.snapshotCh {
		m.writeSnapshot(rec)
	}
}

func (m *Manager) writeSnapshot(rec snapshotRecord) {
	if m.snapshotDir == "" {
		return
	}
	path := m.snapshotPath(rec.ID)
	raw, err := json.Marshal(rec)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

func (m *Manager) snapshotPath(id string) string {
	return filepath.Join(m.snapshotDir, id+".json")
}

func (m *Manager) restoreSnapshots() {
	if m.snapshotDir == "" {
		return
	}
	entries, err := os.ReadDir(m.snapshotDir)
	if err != nil {
		return
	}
	files := make([]os.DirEntry, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		files = append(files, entry)
	}
	if len(files) == 0 {
		return
	}
	sort.Slice(files, func(i, j int) bool {
		infoA, errA := files[i].Info()
		infoB, errB := files[j].Info()
		if errA != nil || errB != nil {
			return files[i].Name() < files[j].Name()
		}
		return infoA.ModTime().After(infoB.ModTime())
	})

	for _, entry := range files {
		if len(m.sessions) >= m.maxSessions {
			return
		}
		path := filepath.Join(m.snapshotDir, entry.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var rec snapshotRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			continue
		}
		if rec.ID == "" {
			continue
		}
		cols := rec.Cols
		rows := rec.Rows
		if cols == 0 {
			cols = 120
		}
		if rows == 0 {
			rows = 32
		}
		var buf []byte
		if rec.Buffer != "" {
			decoded, err := base64.StdEncoding.DecodeString(rec.Buffer)
			if err == nil {
				if rec.Encrypted && rec.Nonce != "" && len(m.snapshotKey) > 0 {
					nonce, err := base64.StdEncoding.DecodeString(rec.Nonce)
					if err == nil {
						plain, err := decryptSnapshot(decoded, nonce, m.snapshotKey)
						if err == nil {
							buf = plain
						}
					}
				} else {
					buf = decoded
				}
			}
		}
		s, err := m.createWithID(rec.ID, rec.Name, cols, rows, buf)
		if err != nil {
			continue
		}
		s.mu.Lock()
		s.LastSnapshot = rec.UpdatedAt
		s.mu.Unlock()
	}
}

func encryptSnapshot(plain []byte, key []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, plain, nil)
	return ciphertext, nonce, nil
}

func decryptSnapshot(ciphertext []byte, nonce []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(nonce) != gcm.NonceSize() {
		return nil, errors.New("invalid snapshot nonce")
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plain, nil
}

func (m *Manager) sweepIdleSessions(ctx context.Context) {
	ticker := time.NewTicker(idleSweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().UTC()
			toClose := make([]string, 0)

			m.mu.RLock()
			for id, s := range m.sessions {
				s.mu.Lock()
				idleFor := now.Sub(s.LastActive)
				s.mu.Unlock()
				if idleFor > m.idleTimeout {
					toClose = append(toClose, id)
				}
			}
			m.mu.RUnlock()

			for _, id := range toClose {
				_ = m.Close(id)
			}
		}
	}
}

func randomID(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func resolveWorkingDir(input string) (string, error) {
	if input == "" || input == "~" {
		return os.UserHomeDir()
	}

	if len(input) > 2 && input[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, input[2:]), nil
	}

	return input, nil
}

func resolveSnapshotDir(input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", nil
	}
	if input == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".webterm", "snapshots"), nil
	}
	if len(input) > 2 && input[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, input[2:]), nil
	}
	return input, nil
}

func resolveCredential(userName string, groupName string) (*syscall.Credential, error) {
	if userName == "" && groupName == "" {
		return nil, nil
	}
	var uid uint32
	var gid uint32
	if userName != "" {
		if parsed, err := strconv.ParseUint(userName, 10, 32); err == nil {
			uid = uint32(parsed)
		} else {
			u, err := user.Lookup(userName)
			if err != nil {
				return nil, err
			}
			parsedUID, err := strconv.ParseUint(u.Uid, 10, 32)
			if err != nil {
				return nil, err
			}
			uid = uint32(parsedUID)
			if groupName == "" {
				parsedGID, err := strconv.ParseUint(u.Gid, 10, 32)
				if err != nil {
					return nil, err
				}
				gid = uint32(parsedGID)
			}
		}
	}
	if groupName != "" {
		if parsed, err := strconv.ParseUint(groupName, 10, 32); err == nil {
			gid = uint32(parsed)
		} else {
			g, err := user.LookupGroup(groupName)
			if err != nil {
				return nil, err
			}
			parsedGID, err := strconv.ParseUint(g.Gid, 10, 32)
			if err != nil {
				return nil, err
			}
			gid = uint32(parsedGID)
		}
	}

	cred := &syscall.Credential{}
	if userName != "" {
		cred.Uid = uid
	}
	if groupName != "" {
		cred.Gid = gid
	}
	return cred, nil
}

func startWithFallbackShells(shell string, workingDir string, env []string, cols uint16, rows uint16, userName string, groupName string) (*os.File, *exec.Cmd, error) {
	paths := make([]string, 0, 3)
	if strings.TrimSpace(shell) != "" {
		paths = append(paths, shell)
	}
	paths = append(paths, "/bin/bash", "/bin/sh")

	var lastErr error
	for _, candidate := range paths {
		path, err := exec.LookPath(candidate)
		if err != nil {
			lastErr = err
			continue
		}
		ptmx, cmd, err := startShell(path, workingDir, env, cols, rows, userName, groupName, true)
		if err == nil {
			return ptmx, cmd, nil
		}
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			ptmx, cmd, retryErr := startShell(path, workingDir, env, cols, rows, userName, groupName, false)
			if retryErr == nil {
				return ptmx, cmd, nil
			}
			lastErr = retryErr
			continue
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no usable shell found")
	}
	return nil, nil, lastErr
}

func startShell(path string, workingDir string, env []string, cols uint16, rows uint16, userName string, groupName string, setpgid bool) (*os.File, *exec.Cmd, error) {
	cmd := exec.Command(path)
	cmd.Dir = workingDir
	if setpgid {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if userName != "" || groupName != "" {
		cred, err := resolveCredential(userName, groupName)
		if err != nil {
			return nil, nil, err
		}
		if cmd.SysProcAttr == nil {
			cmd.SysProcAttr = &syscall.SysProcAttr{}
		}
		cmd.SysProcAttr.Credential = cred
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, nil, err
	}
	return ptmx, cmd, nil
}
