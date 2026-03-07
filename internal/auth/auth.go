package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"webterm/internal/config"
)

const (
	SessionCookieName = "webterm_session"
	CSRFCookieName    = "webterm_csrf"
)

type Session struct {
	ID        string
	CSRFToken string
	ExpiresAt time.Time
}

type Manager struct {
	mode         string
	passwordHash string
	token        string
	ttl          time.Duration

	mu       sync.RWMutex
	sessions map[string]Session
}

func New(cfg config.AuthConfig) (*Manager, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "password"
	}

	if mode != "password" && mode != "token" {
		return nil, errors.New("auth mode must be password or token")
	}

	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 24 * time.Hour
	}

	if mode == "password" && strings.TrimSpace(cfg.PasswordHash) == "" {
		return nil, errors.New("password mode requires auth.password_hash")
	}
	if mode == "token" && strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("token mode requires auth.token")
	}

	return &Manager{
		mode:         mode,
		passwordHash: cfg.PasswordHash,
		token:        cfg.Token,
		ttl:          cfg.SessionTTL,
		sessions:     map[string]Session{},
	}, nil
}

func (m *Manager) Mode() string {
	return m.mode
}

func (m *Manager) Authenticate(secret string) (Session, error) {
	secret = strings.TrimSpace(secret)
	switch m.mode {
	case "password":
		if err := bcrypt.CompareHashAndPassword([]byte(m.passwordHash), []byte(secret)); err != nil {
			return Session{}, errors.New("invalid credentials")
		}
	case "token":
		if subtle.ConstantTimeCompare([]byte(secret), []byte(m.token)) != 1 {
			return Session{}, errors.New("invalid credentials")
		}
	default:
		return Session{}, errors.New("unsupported auth mode")
	}

	sid, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}
	csrf, err := randomToken(32)
	if err != nil {
		return Session{}, err
	}

	now := time.Now().UTC()
	session := Session{
		ID:        sid,
		CSRFToken: csrf,
		ExpiresAt: now.Add(m.ttl),
	}

	m.mu.Lock()
	m.sessions[session.ID] = session
	m.mu.Unlock()

	return session, nil
}

func (m *Manager) GetSession(id string) (Session, bool) {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()
	if !ok {
		return Session{}, false
	}
	if time.Now().UTC().After(s.ExpiresAt) {
		m.DeleteSession(id)
		return Session{}, false
	}
	return s, true
}

func (m *Manager) DeleteSession(id string) {
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
}

func (m *Manager) SetSessionCookies(w http.ResponseWriter, r *http.Request, session Session) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    session.ID,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.ExpiresAt,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    session.CSRFToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.ExpiresAt,
	})
}

func (m *Manager) ClearSessionCookies(w http.ResponseWriter, r *http.Request) {
	secure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: false,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
