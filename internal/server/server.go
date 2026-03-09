package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"webterm/frontend"
	"webterm/internal/auth"
	"webterm/internal/config"
	"webterm/internal/monitoring"
	"webterm/internal/terminal"

	"github.com/gorilla/websocket"
)

const (
	streamReadBufferSize = 32 * 1024
)

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  streamReadBufferSize,
	WriteBufferSize: streamReadBufferSize,
	CheckOrigin: func(r *http.Request) bool {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin == "" {
			return true
		}
		parsed, err := url.Parse(origin)
		if err != nil {
			return false
		}
		return strings.EqualFold(parsed.Host, r.Host)
	},
}

type terminalManager interface {
	List() []*terminal.Session
	Get(id string) (*terminal.Session, bool)
	Create(name string, cols uint16, rows uint16) (*terminal.Session, error)
	Rename(id string, name string) (*terminal.Session, error)
	Close(id string) error
	CloseAll()
	Read(id string, buf []byte) (int, error)
	Write(id string, data []byte) (int, error)
	Resize(id string, cols uint16, rows uint16) error
	History(id string) ([]byte, error)
	Snapshot(id string) ([]byte, uint16, uint16, bool, error)
}

type app struct {
	auth         *auth.Manager
	terminals    terminalManager
	limiter      *loginLimiter
	streamMu     sync.Mutex
	streamCancel map[string]context.CancelFunc
	idleTimeout  time.Duration
	monitoring   *monitoring.Manager
	monitorToken string
}

type contextKey string

const sessionContextKey contextKey = "session"

type loginLimiter struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	attempts map[string]int
	resetAt  map[string]time.Time
}

func newLoginLimiter(max int, window time.Duration) *loginLimiter {
	if max <= 0 {
		max = 10
	}
	if window <= 0 {
		window = 10 * time.Minute
	}
	return &loginLimiter{
		max:      max,
		window:   window,
		attempts: map[string]int{},
		resetAt:  map[string]time.Time{},
	}
}

func (l *loginLimiter) allow(key string) bool {
	if l == nil {
		return true
	}
	now := time.Now().UTC()
	l.mu.Lock()
	defer l.mu.Unlock()
	reset, ok := l.resetAt[key]
	if !ok || now.After(reset) {
		l.resetAt[key] = now.Add(l.window)
		l.attempts[key] = 0
	}
	if l.attempts[key] >= l.max {
		return false
	}
	l.attempts[key]++
	return true
}

func (l *loginLimiter) reset(key string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
	delete(l.resetAt, key)
}

func clientIP(r *http.Request) string {
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func Run(ctx context.Context, cfg config.Config) error {
	authManager, err := auth.New(cfg.Auth)
	if err != nil {
		return err
	}

	terminalManager, err := terminal.New(cfg)
	if err != nil {
		return err
	}

	var monitoringManager *monitoring.Manager
	if cfg.Monitoring.Enabled {
		monitorPath, pathErr := resolveHomePath(cfg.Monitoring.DBPath)
		if pathErr != nil {
			return pathErr
		}
		store, storeErr := monitoring.OpenStore(monitorPath)
		if storeErr != nil {
			return storeErr
		}
		monitoringManager = monitoring.NewManager(store, cfg.Monitoring.Retention)
	}

	application := &app{
		auth:         authManager,
		terminals:    terminalManager,
		limiter:      newLoginLimiter(cfg.Auth.RateLimitMax, cfg.Auth.RateLimitWindow),
		streamCancel: map[string]context.CancelFunc{},
		idleTimeout:  cfg.Sessions.IdleTimeout,
		monitoring:   monitoringManager,
		monitorToken: cfg.Monitoring.Token,
	}
	if application.idleTimeout <= 0 {
		application.idleTimeout = 24 * time.Hour
	}

	defer terminalManager.CloseAll()
	if monitoringManager != nil {
		defer monitoringManager.Store().Close()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", healthHandler)
	mux.HandleFunc("/api/login", application.loginHandler)
	mux.Handle("/api/logout", application.withAuth(application.withCSRF(http.HandlerFunc(application.logoutHandler))))
	mux.Handle("/api/me", application.withAuth(http.HandlerFunc(application.meHandler)))
	mux.Handle("/api/terminal/sessions", application.withAuth(http.HandlerFunc(application.sessionsHandler)))
	mux.Handle("/api/terminal/sessions/", application.withAuth(http.HandlerFunc(application.sessionByIDHandler)))
	mux.Handle("/api/terminal/ws/", application.withAuth(http.HandlerFunc(application.terminalWebsocketHandler)))
	mux.Handle("/api/terminal/input/", application.withAuth(application.withCSRF(http.HandlerFunc(application.terminalInputHandler))))
	mux.Handle("/api/terminal/resize/", application.withAuth(application.withCSRF(http.HandlerFunc(application.terminalResizeHandler))))
	mux.Handle("/api/monitoring/v1/ingest", http.HandlerFunc(application.monitoringIngestHandler))
	mux.Handle("/api/monitoring/v1/sessions", http.HandlerFunc(application.monitoringSessionsHandler))
	mux.Handle("/api/monitoring/v1/summary", http.HandlerFunc(application.monitoringSummaryHandler))
	mux.Handle("/api/monitoring/v1/stream", http.HandlerFunc(application.monitoringStreamHandler))
	mux.Handle("/api/monitoring/v1/notify", http.HandlerFunc(application.monitoringNotifyHandler))
	mux.Handle("/api/monitoring/v1/events", http.HandlerFunc(application.monitoringEventsHandler))
	mux.Handle("/api/monitoring/v1/logbook/", http.HandlerFunc(application.monitoringLogbookHandler))

	frontendFS, err := fs.Sub(frontend.Dist, frontend.RootDir)
	if err != nil {
		return err
	}
	if frontend.RootDir == "embed" {
		if info, statErr := os.Stat("frontend/out"); statErr == nil && info.IsDir() {
			frontendFS = os.DirFS("frontend/out")
		}
	}
	fileServer := http.FileServer(http.FS(frontendFS))
	mux.Handle("/", spaFallback(fileServer, frontendFS))

	addr := net.JoinHostPort(cfg.Server.Bind, strconv.Itoa(cfg.Server.Port))
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           withSecurityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("http server listening on %s", addr)
		if serveErr := httpServer.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case serveErr := <-errCh:
		return serveErr
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "name": "webterm"})
}

func (a *app) loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ip := clientIP(r)
	if !a.limiter.allow(ip) {
		http.Error(w, "too many login attempts", http.StatusTooManyRequests)
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	session, err := a.auth.Authenticate(req.Password)
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	a.limiter.reset(ip)

	a.auth.SetSessionCookies(w, r, session)
	writeJSON(w, http.StatusOK, map[string]string{
		"status":     "ok",
		"auth_mode":  a.auth.Mode(),
		"csrf_token": session.CSRFToken,
	})
}

func (a *app) logoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	a.auth.DeleteSession(session.ID)
	a.auth.ClearSessionCookies(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *app) meHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated":        true,
		"expires_at":           session.ExpiresAt,
		"auth_mode":            a.auth.Mode(),
		"idle_timeout_seconds": int64(a.idleTimeout.Seconds()),
	})
}

func (a *app) sessionsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		all := a.terminals.List()
		type item struct {
			ID         string    `json:"id"`
			Name       string    `json:"name"`
			CreatedAt  time.Time `json:"created_at"`
			LastActive time.Time `json:"last_active"`
		}
		out := make([]item, 0, len(all))
		for _, s := range all {
			out = append(out, item{ID: s.ID, Name: s.Name, CreatedAt: s.CreatedAt, LastActive: s.LastActive})
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
			Cols uint16 `json:"cols"`
			Rows uint16 `json:"rows"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		if req.Cols == 0 {
			req.Cols = 120
		}
		if req.Rows == 0 {
			req.Rows = 32
		}
		s, err := a.terminals.Create(req.Name, req.Cols, req.Rows)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if a.monitoring != nil {
			_ = a.monitoring.Notify(r.Context(), monitoring.Event{
				SessionID: s.ID,
				Type:      "info",
				Title:     "Session started",
				Message:   "New session created",
				Timestamp: time.Now().UTC(),
			}, &monitoring.SessionSummary{
				ID:           s.ID,
				Name:         s.Name,
				Command:      "",
				ProcessID:    0,
				Status:       "running",
				Attention:    "low",
				LastActivity: s.LastActive,
				CPUPercent:   0,
			})
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"id":         s.ID,
			"name":       s.Name,
			"created_at": s.CreatedAt,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) sessionByIDHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/terminal/sessions/")
	id = strings.TrimSpace(id)
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := a.terminals.Close(id); err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "closed"})
	case http.MethodPatch:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			http.Error(w, "name required", http.StatusBadRequest)
			return
		}
		s, err := a.terminals.Rename(id, name)
		if err != nil {
			http.Error(w, "session not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":   s.ID,
			"name": s.Name,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) terminalWebsocketHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	session := sessionFromContext(r.Context())
	csrf := strings.TrimSpace(r.URL.Query().Get("csrf"))
	if csrf == "" || csrf != session.CSRFToken {
		http.Error(w, "csrf validation failed", http.StatusForbidden)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/terminal/ws/")
	id = strings.TrimSpace(id)
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}

	if _, ok := a.terminals.Get(id); !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	a.streamMu.Lock()
	if prevCancel, ok := a.streamCancel[id]; ok {
		prevCancel()
	}
	a.streamCancel[id] = cancel
	a.streamMu.Unlock()
	defer func() {
		a.streamMu.Lock()
		delete(a.streamCancel, id)
		a.streamMu.Unlock()
		cancel()
	}()

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	if colsStr := strings.TrimSpace(r.URL.Query().Get("cols")); colsStr != "" {
		if rowsStr := strings.TrimSpace(r.URL.Query().Get("rows")); rowsStr != "" {
			cols, colsErr := strconv.Atoi(colsStr)
			rows, rowsErr := strconv.Atoi(rowsStr)
			if colsErr == nil && rowsErr == nil && cols > 0 && rows > 0 {
				_ = a.terminals.Resize(id, uint16(cols), uint16(rows))
			}
		}
	}

	if snap, cols, rows, ok, err := a.terminals.Snapshot(id); err == nil && ok && len(snap) > 0 {
		encoded := base64.StdEncoding.EncodeToString(snap)
		payload := map[string]any{
			"type": "snapshot",
			"data": encoded,
			"cols": cols,
			"rows": rows,
			"tmux": true,
		}
		if err := sendWSJSON(conn, payload); err != nil {
			return
		}
	} else if history, err := a.terminals.History(id); err == nil && len(history) > 0 {
		encoded := base64.StdEncoding.EncodeToString(history)
		payload := map[string]any{
			"type": "snapshot",
			"data": encoded,
		}
		if session, ok := a.terminals.Get(id); ok {
			payload["cols"] = session.LastCols
			payload["rows"] = session.LastRows
			payload["updated_at"] = session.LastSnapshot
		}
		if err := sendWSJSON(conn, payload); err != nil {
			return
		}
	}

	readErrCh := make(chan error, 1)
	go func() {
		for {
			messageType, payload, err := conn.ReadMessage()
			if err != nil {
				readErrCh <- err
				return
			}
			switch messageType {
			case websocket.TextMessage:
				a.handleWebsocketMessage(id, payload)
			case websocket.BinaryMessage:
				if len(payload) > 0 {
					if _, err := a.terminals.Write(id, payload); err != nil {
						log.Printf("terminal websocket input error: %v", err)
					}
				}
			}
		}
	}()

	buf := make([]byte, streamReadBufferSize)
	for {
		select {
		case <-ctx.Done():
			return
		case <-readErrCh:
			return
		default:
		}

		n, readErr := a.terminals.Read(id, buf)
		if n > 0 {
			if err := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
				return
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				log.Printf("terminal websocket read error: %v", readErr)
			}
			return
		}
	}
}

func (a *app) handleWebsocketMessage(id string, payload []byte) {
	var msg struct {
		Type string `json:"type"`
		Data string `json:"data"`
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}
	if err := json.Unmarshal(payload, &msg); err != nil || msg.Type == "" {
		if len(payload) == 0 {
			return
		}
		if _, err := a.terminals.Write(id, payload); err != nil {
			log.Printf("terminal websocket input error: %v", err)
		}
		return
	}

	switch msg.Type {
	case "input":
		if msg.Data == "" {
			return
		}
		if _, err := a.terminals.Write(id, []byte(msg.Data)); err != nil {
			log.Printf("terminal websocket input error: %v", err)
		}
	case "resize":
		if msg.Cols == 0 || msg.Rows == 0 {
			return
		}
		if err := a.terminals.Resize(id, msg.Cols, msg.Rows); err != nil {
			log.Printf("terminal websocket resize error: %v", err)
		}
	}
}

func sendWSJSON(conn *websocket.Conn, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, raw)
}

func (a *app) terminalInputHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/terminal/input/")
	id = strings.TrimSpace(id)
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}

	if _, ok := a.terminals.Get(id); !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	if len(body) == 0 {
		http.Error(w, "empty input", http.StatusBadRequest)
		return
	}

	if _, err := a.terminals.Write(id, body); err != nil {
		http.Error(w, "failed to write input", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *app) terminalResizeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/terminal/resize/")
	id = strings.TrimSpace(id)
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}

	if _, ok := a.terminals.Get(id); !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	var req struct {
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	if req.Cols == 0 || req.Rows == 0 {
		http.Error(w, "invalid size", http.StatusBadRequest)
		return
	}

	if err := a.terminals.Resize(id, req.Cols, req.Rows); err != nil {
		http.Error(w, "resize failed", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *app) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(auth.SessionCookieName)
		if err != nil || strings.TrimSpace(cookie.Value) == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		session, ok := a.auth.GetSession(cookie.Value)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), sessionContextKey, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *app) withCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := sessionFromContext(r.Context())
		headerToken := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
		if headerToken == "" || headerToken != session.CSRFToken {
			http.Error(w, "csrf validation failed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sessionFromContext(ctx context.Context) auth.Session {
	session, _ := ctx.Value(sessionContextKey).(auth.Session)
	return session
}

func spaFallback(fileServer http.Handler, staticFS fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}

		cleanPath := strings.TrimPrefix(r.URL.Path, "/")
		if cleanPath != "" {
			if _, err := fs.Stat(staticFS, cleanPath); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		index, err := fs.ReadFile(staticFS, "index.html")
		if err != nil {
			http.Error(w, "frontend assets missing", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	})
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
