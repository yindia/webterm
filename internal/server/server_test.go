package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"webterm/internal/auth"
	"webterm/internal/config"
	"webterm/internal/terminal"
)

type fakeTerminalManager struct {
	mu       sync.Mutex
	sessions map[string]*terminal.Session
	history  map[string][]byte
	readData map[string][]byte
}

func newFakeTerminalManager() *fakeTerminalManager {
	return &fakeTerminalManager{
		sessions: map[string]*terminal.Session{},
		history:  map[string][]byte{},
		readData: map[string][]byte{},
	}
}

func (f *fakeTerminalManager) List() []*terminal.Session {
	f.mu.Lock()
	defer f.mu.Unlock()
	list := make([]*terminal.Session, 0, len(f.sessions))
	for _, s := range f.sessions {
		list = append(list, s)
	}
	return list
}

func (f *fakeTerminalManager) Get(id string) (*terminal.Session, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	return s, ok
}

func (f *fakeTerminalManager) Create(name string, cols uint16, rows uint16) (*terminal.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := &terminal.Session{ID: "s1", Name: name, CreatedAt: time.Now(), LastActive: time.Now(), LastCols: cols, LastRows: rows}
	f.sessions[s.ID] = s
	return s, nil
}

func (f *fakeTerminalManager) Rename(id string, name string) (*terminal.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}
	s.Name = name
	return s, nil
}

func (f *fakeTerminalManager) Close(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sessions[id]; !ok {
		return errors.New("session not found")
	}
	delete(f.sessions, id)
	return nil
}

func (f *fakeTerminalManager) CloseAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = map[string]*terminal.Session{}
}

func (f *fakeTerminalManager) Read(id string, buf []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data := f.readData[id]
	if len(data) == 0 {
		return 0, io.EOF
	}
	n := copy(buf, data)
	f.readData[id] = nil
	return n, io.EOF
}

func (f *fakeTerminalManager) Write(id string, data []byte) (int, error) {
	if _, ok := f.sessions[id]; !ok {
		return 0, errors.New("session not found")
	}
	return len(data), nil
}

func (f *fakeTerminalManager) Resize(id string, cols uint16, rows uint16) error {
	if _, ok := f.sessions[id]; !ok {
		return errors.New("session not found")
	}
	return nil
}

func (f *fakeTerminalManager) History(id string) ([]byte, error) {
	if data, ok := f.history[id]; ok {
		return data, nil
	}
	return nil, nil
}

func (f *fakeTerminalManager) Snapshot(id string) ([]byte, uint16, uint16, bool, error) {
	return nil, 0, 0, false, nil
}

func attachCookies(rec *httptest.ResponseRecorder, req *http.Request) {
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
}

func newTestApp(t *testing.T) *app {
	t.Helper()
	cfg := config.Default()
	cfg.Auth.Mode = "password"
	cfg.Auth.Password = "test-token"
	cfg.Sessions.SnapshotDir = ""

	authManager, err := auth.New(cfg.Auth)
	if err != nil {
		t.Fatalf("auth.New failed: %v", err)
	}
	termManager, err := terminal.New(cfg)
	if err != nil {
		t.Fatalf("terminal.New failed: %v", err)
	}
	return &app{
		auth:         authManager,
		terminals:    termManager,
		limiter:      newLoginLimiter(2, time.Minute),
		streamCancel: map[string]context.CancelFunc{},
		idleTimeout:  cfg.Sessions.IdleTimeout,
	}
}

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	healthHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestHealthHandlerRejectsMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/health", nil)
	rec := httptest.NewRecorder()
	healthHandler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestMeHandlerRejectsMethod(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodPost, "/api/me", nil)
	rec := httptest.NewRecorder()
	a.meHandler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	body := `{"password":"bad"}`
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	a.loginHandler(rec, req)

	req2 := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	rec2 := httptest.NewRecorder()
	a.loginHandler(rec2, req2)

	req3 := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	rec3 := httptest.NewRecorder()
	a.loginHandler(rec3, req3)

	if rec3.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec3.Code)
	}
}

func TestLoginRejectsMethod(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodGet, "/api/login", nil)
	rec := httptest.NewRecorder()
	a.loginHandler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestLoginRejectsInvalidJSON(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader("{"))
	rec := httptest.NewRecorder()
	a.loginHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestLoginSuccessAfterFailure(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	bad := `{"password":"bad"}`
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(bad))
	rec := httptest.NewRecorder()
	a.loginHandler(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	good := `{"password":"test-token"}`
	req2 := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(good))
	rec2 := httptest.NewRecorder()
	a.loginHandler(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
}

func TestLogoutRejectsMethod(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodGet, "/api/logout", nil)
	rec := httptest.NewRecorder()
	a.logoutHandler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestSessionsHandlerRejectsMethod(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodPut, "/api/terminal/sessions", nil)
	rec := httptest.NewRecorder()
	a.sessionsHandler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestSessionsHandlerPostCreates(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodPost, "/api/terminal/sessions", strings.NewReader(`{"name":"one","cols":80,"rows":24}`))
	rec := httptest.NewRecorder()
	a.sessionsHandler(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestSessionByIDHandlerDelete(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	s, err := a.terminals.Create("one", 80, 24)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/terminal/sessions/"+s.ID, nil)
	rec := httptest.NewRecorder()
	a.sessionByIDHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestSessionByIDHandlerRename(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	s, err := a.terminals.Create("one", 80, 24)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/terminal/sessions/"+s.ID, strings.NewReader(`{"name":"renamed"}`))
	rec := httptest.NewRecorder()
	a.sessionByIDHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if body.ID != s.ID || body.Name != "renamed" {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestSessionByIDHandlerRenameRequiresName(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	s, err := a.terminals.Create("one", 80, 24)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/terminal/sessions/"+s.ID, strings.NewReader(`{"name":""}`))
	rec := httptest.NewRecorder()
	a.sessionByIDHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestSessionByIDHandlerNotFound(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodDelete, "/api/terminal/sessions/missing", nil)
	rec := httptest.NewRecorder()
	a.sessionByIDHandler(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestSessionByIDHandlerRejectsMethod(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodPut, "/api/terminal/sessions/abc", nil)
	rec := httptest.NewRecorder()
	a.sessionByIDHandler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestSessionByIDHandlerMissingID(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodDelete, "/api/terminal/sessions/", nil)
	rec := httptest.NewRecorder()
	a.sessionByIDHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestTerminalInputHandlerRejectsMethod(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodGet, "/api/terminal/input/abc", nil)
	rec := httptest.NewRecorder()
	a.terminalInputHandler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestTerminalResizeHandlerRejectsMethod(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodGet, "/api/terminal/resize/abc", nil)
	rec := httptest.NewRecorder()
	a.terminalResizeHandler(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestTerminalInputHandlerMissingSession(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodPost, "/api/terminal/input/missing", strings.NewReader("ls"))
	rec := httptest.NewRecorder()
	a.terminalInputHandler(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestTerminalInputHandlerOK(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	s, err := a.terminals.Create("one", 80, 24)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/terminal/input/"+s.ID, strings.NewReader("ls\n"))
	rec := httptest.NewRecorder()
	a.terminalInputHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestTerminalInputHandlerMissingID(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodPost, "/api/terminal/input/", strings.NewReader("ls"))
	rec := httptest.NewRecorder()
	a.terminalInputHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestTerminalResizeHandlerInvalidBody(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodPost, "/api/terminal/resize/missing", strings.NewReader("{"))
	rec := httptest.NewRecorder()
	a.terminalResizeHandler(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestTerminalResizeHandlerOK(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	s, err := a.terminals.Create("one", 80, 24)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/terminal/resize/"+s.ID, strings.NewReader(`{"cols":90,"rows":30}`))
	rec := httptest.NewRecorder()
	a.terminalResizeHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestTerminalResizeHandlerMissingID(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodPost, "/api/terminal/resize/", strings.NewReader(`{"cols":90,"rows":30}`))
	rec := httptest.NewRecorder()
	a.terminalResizeHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestWithSecurityHeaders(t *testing.T) {
	h := withSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("X-Content-Type-Options") == "" {
		t.Fatalf("expected security headers")
	}
	if rec.Header().Get("X-Frame-Options") == "" {
		t.Fatalf("expected frame options")
	}
}

func TestSpaFallbackNotFoundServesIndex(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("ok"), 0o600); err != nil {
		t.Fatalf("write index failed: %v", err)
	}
	fsys := os.DirFS(dir)
	h := spaFallback(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}), fsys)

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestSpaFallbackServesExistingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("ok"), 0o600); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	fSys := os.DirFS(dir)
	fileServer := http.FileServer(http.FS(fSys))
	h := spaFallback(fileServer, fSys)

	req := httptest.NewRequest(http.MethodGet, "/file.txt", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestLoginLimiterReset(t *testing.T) {
	limiter := newLoginLimiter(1, time.Millisecond)
	key := "127.0.0.1"
	if !limiter.allow(key) {
		t.Fatalf("expected allow")
	}
	if limiter.allow(key) {
		t.Fatalf("expected deny")
	}
	limiter.reset(key)
	if !limiter.allow(key) {
		t.Fatalf("expected allow after reset")
	}
}

func TestNewLoginLimiterDefaults(t *testing.T) {
	limiter := newLoginLimiter(0, 0)
	if limiter.max <= 0 {
		t.Fatalf("expected default max")
	}
	if limiter.window <= 0 {
		t.Fatalf("expected default window")
	}
}

func TestLoginSuccessAndMe(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	body := `{"password":"test-token"}`
	req := httptest.NewRequest(http.MethodPost, "/api/login", strings.NewReader(body))
	rec := httptest.NewRecorder()
	a.loginHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	resp := rec.Result()
	if len(resp.Cookies()) == 0 {
		t.Fatalf("expected cookies to be set")
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	attachCookies(rec, meReq)
	meRec := httptest.NewRecorder()
	a.withAuth(http.HandlerFunc(a.meHandler)).ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", meRec.Code)
	}
}

func TestSessionsHandlerGetEmpty(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	session, err := a.auth.Authenticate("test-token")
	if err != nil {
		t.Fatalf("auth failed: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/terminal/sessions", nil)
	recCookies := httptest.NewRecorder()
	a.auth.SetSessionCookies(recCookies, req, session)
	attachCookies(recCookies, req)
	rec := httptest.NewRecorder()
	a.withAuth(http.HandlerFunc(a.sessionsHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
}

func TestSessionsHandlerGetWithItem(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	if _, err := a.terminals.Create("one", 80, 24); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/terminal/sessions", nil)
	rec := httptest.NewRecorder()
	a.sessionsHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
}

func TestWithCSRF(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	session, err := a.auth.Authenticate("test-token")
	if err != nil {
		t.Fatalf("auth failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	rec := httptest.NewRecorder()
	a.auth.SetSessionCookies(rec, req, session)
	attachCookies(rec, req)
	req.Header.Set("X-CSRF-Token", session.CSRFToken)

	wrap := a.withAuth(a.withCSRF(http.HandlerFunc(a.logoutHandler)))
	wrap.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestWithAuthRejectsMissingSession(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()
	a.withAuth(http.HandlerFunc(a.meHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestWithAuthRejectsExpiredSession(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.Mode = "password"
	cfg.Auth.Password = "test-token"
	cfg.Auth.SessionTTL = 1 * time.Nanosecond
	cfg.Sessions.SnapshotDir = ""

	authManager, err := auth.New(cfg.Auth)
	if err != nil {
		t.Fatalf("auth.New failed: %v", err)
	}
	termManager, err := terminal.New(cfg)
	if err != nil {
		t.Fatalf("terminal.New failed: %v", err)
	}
	a := &app{
		auth:         authManager,
		terminals:    termManager,
		limiter:      newLoginLimiter(2, time.Minute),
		streamCancel: map[string]context.CancelFunc{},
		idleTimeout:  cfg.Sessions.IdleTimeout,
	}
	defer termManager.CloseAll()

	session, err := authManager.Authenticate("test-token")
	if err != nil {
		t.Fatalf("auth failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()
	authManager.SetSessionCookies(rec, req, session)
	attachCookies(rec, req)

	wrap := a.withAuth(http.HandlerFunc(a.meHandler))
	wrap.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSessionsHandlerPostInvalidJSON(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	req := httptest.NewRequest(http.MethodPost, "/api/terminal/sessions", strings.NewReader("{"))
	rec := httptest.NewRecorder()
	a.sessionsHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestTerminalInputHandlerEmptyBody(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	s, err := a.terminals.Create("one", 80, 24)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/terminal/input/"+s.ID, strings.NewReader(""))
	rec := httptest.NewRecorder()
	a.terminalInputHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestTerminalResizeHandlerInvalidSize(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	s, err := a.terminals.Create("one", 80, 24)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/terminal/resize/"+s.ID, strings.NewReader(`{"cols":0,"rows":0}`))
	rec := httptest.NewRecorder()
	a.terminalResizeHandler(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestWithCSRFRejectsMissingToken(t *testing.T) {
	a := newTestApp(t)
	defer a.terminals.CloseAll()

	session, err := a.auth.Authenticate("test-token")
	if err != nil {
		t.Fatalf("auth failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/logout", nil)
	rec := httptest.NewRecorder()
	a.auth.SetSessionCookies(rec, req, session)
	attachCookies(rec, req)

	wrap := a.withAuth(a.withCSRF(http.HandlerFunc(a.logoutHandler)))
	wrap.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	ip := clientIP(req)
	if ip != "10.0.0.1" {
		t.Fatalf("expected remote addr host")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	ip2 := clientIP(req2)
	if ip2 != "1.2.3.4" {
		t.Fatalf("expected forwarded ip")
	}

	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.RemoteAddr = "bad"
	ip3 := clientIP(req3)
	if ip3 == "" {
		t.Fatalf("expected fallback ip")
	}
}

func TestRunShutdown(t *testing.T) {
	cfg := config.Default()
	cfg.Server.Bind = "127.0.0.1"
	cfg.Server.Port = 0
	cfg.Auth.Mode = "password"
	cfg.Auth.Password = "test-token"
	cfg.Sessions.SnapshotDir = ""

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, cfg)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not exit")
	}
}
