package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"webterm/internal/config"
)

func TestPasswordAuthSuccess(t *testing.T) {
	mgr, err := New(config.AuthConfig{Mode: "password", Password: "secret", SessionTTL: time.Hour})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	s, err := mgr.Authenticate("secret")
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
	if s.ID == "" || s.CSRFToken == "" {
		t.Fatalf("expected non-empty session id and csrf token")
	}
}

func TestPasswordAuthRejectsInvalidSecret(t *testing.T) {
	mgr, err := New(config.AuthConfig{Mode: "password", Password: "secret", SessionTTL: time.Hour})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	if _, err := mgr.Authenticate("wrong"); err == nil {
		t.Fatalf("expected invalid credentials error")
	}
}

func TestAuthenticateTrimsSecret(t *testing.T) {
	mgr, err := New(config.AuthConfig{Mode: "password", Password: "secret", SessionTTL: time.Hour})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if _, err := mgr.Authenticate("  secret  "); err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
}

func TestNewRejectsInvalidMode(t *testing.T) {
	if _, err := New(config.AuthConfig{Mode: "unknown"}); err == nil {
		t.Fatalf("expected error for invalid mode")
	}
	if _, err := New(config.AuthConfig{Mode: "token"}); err == nil {
		t.Fatalf("expected error for invalid mode")
	}
}

func TestNewRejectsMissingPasswordHash(t *testing.T) {
	if _, err := New(config.AuthConfig{Mode: "password"}); err == nil {
		t.Fatalf("expected error for missing password")
	}
}

func TestPasswordAuthPlainSuccess(t *testing.T) {
	mgr, err := New(config.AuthConfig{Mode: "password", Password: "secret", SessionTTL: time.Hour})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if _, err := mgr.Authenticate("secret"); err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
}

func TestPasswordAuthPlainRejectsInvalidSecret(t *testing.T) {
	mgr, err := New(config.AuthConfig{Mode: "password", Password: "secret", SessionTTL: time.Hour})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if _, err := mgr.Authenticate("wrong"); err == nil {
		t.Fatalf("expected invalid credentials error")
	}
}

func TestSessionExpires(t *testing.T) {
	mgr, err := New(config.AuthConfig{Mode: "password", Password: "secret", SessionTTL: 1 * time.Nanosecond})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	s, err := mgr.Authenticate("secret")
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, ok := mgr.GetSession(s.ID); ok {
		t.Fatalf("expected session to expire")
	}
}

func TestMode(t *testing.T) {
	mgr, err := New(config.AuthConfig{Mode: "password", Password: "abc", SessionTTL: time.Hour})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if mgr.Mode() != "password" {
		t.Fatalf("expected password mode")
	}
}

func TestRandomTokenLength(t *testing.T) {
	val, err := randomToken(16)
	if err != nil {
		t.Fatalf("randomToken failed: %v", err)
	}
	if len(val) != 32 {
		t.Fatalf("expected hex length 32")
	}
}

func TestSessionCookieLifecycle(t *testing.T) {
	mgr, err := New(config.AuthConfig{Mode: "password", Password: "secret", SessionTTL: time.Hour})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	session, err := mgr.Authenticate("secret")
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
	mgr.SetSessionCookies(rec, req, session)

	resp := rec.Result()
	if len(resp.Cookies()) < 2 {
		t.Fatalf("expected session + csrf cookies")
	}

	if _, ok := mgr.GetSession(session.ID); !ok {
		t.Fatalf("expected session to exist")
	}

	mgr.DeleteSession(session.ID)
	if _, ok := mgr.GetSession(session.ID); ok {
		t.Fatalf("expected session to be deleted")
	}

	rec2 := httptest.NewRecorder()
	mgr.ClearSessionCookies(rec2, req)
	resp2 := rec2.Result()
	if len(resp2.Cookies()) < 2 {
		t.Fatalf("expected cleared cookies")
	}
}
