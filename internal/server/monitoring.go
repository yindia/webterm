package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"webterm/internal/auth"
	"webterm/internal/monitoring"
)

func resolveHomePath(input string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", errors.New("path required")
	}
	if input == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if strings.HasPrefix(input, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home + input[1:], nil
	}
	return input, nil
}

func (a *app) monitoringEnabled() bool {
	return a.monitoring != nil
}

func (a *app) monitoringIngestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.monitoringEnabled() {
		http.Error(w, "monitoring disabled", http.StatusNotFound)
		return
	}
	if !a.allowMonitorRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req struct {
		Sessions []monitoring.SessionSummary `json:"sessions"`
		Samples  []monitoring.ActivitySample `json:"samples"`
		Events   []monitoring.Event          `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if err := a.monitoring.Ingest(ctx, req.Sessions, req.Samples, req.Events); err != nil {
		http.Error(w, "ingest failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *app) monitoringSessionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.monitoringEnabled() {
		http.Error(w, "monitoring disabled", http.StatusNotFound)
		return
	}
	if !a.allowMonitorRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	all := a.terminals.List()
	type item struct {
		ID         string    `json:"id"`
		Name       string    `json:"name"`
		LastActive time.Time `json:"last_active"`
	}
	out := make([]item, 0, len(all))
	for _, s := range all {
		out = append(out, item{ID: s.ID, Name: s.Name, LastActive: s.LastActive})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func (a *app) monitoringSummaryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.monitoringEnabled() {
		http.Error(w, "monitoring disabled", http.StatusNotFound)
		return
	}
	if !a.allowMonitorRequest(r) && !a.isAuthenticated(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ctx := r.Context()
	summaries, err := a.monitoring.Store().GetSummaries(ctx)
	if err != nil {
		http.Error(w, "failed to load summary", http.StatusInternalServerError)
		return
	}
	if len(summaries) == 0 {
		for _, s := range a.terminals.List() {
			summaries = append(summaries, monitoring.SessionSummary{
				ID:           s.ID,
				Name:         s.Name,
				Command:      "",
				ProcessID:    0,
				Status:       "running",
				Attention:    "low",
				LastActivity: s.LastActive,
				CPUPercent:   0,
				MemoryBytes:  0,
				GPUUtil:      -1,
			})
		}
	}
	limit := 30
	if raw := r.URL.Query().Get("activity_limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}

	type sessionResponse struct {
		monitoring.SessionSummary
		Activity []monitoring.ActivityPoint `json:"activity"`
	}

	out := make([]sessionResponse, 0, len(summaries))
	for _, summary := range summaries {
		activity, err := a.monitoring.Store().GetActivitySeries(ctx, summary.ID, limit)
		if err != nil {
			activity = nil
		}
		out = append(out, sessionResponse{SessionSummary: summary, Activity: activity})
	}

	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func (a *app) monitoringStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.monitoringEnabled() {
		http.Error(w, "monitoring disabled", http.StatusNotFound)
		return
	}
	if !a.allowMonitorRequest(r) && !a.isAuthenticated(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	chID, ch := a.monitoring.Hub().Subscribe()
	defer a.monitoring.Hub().Unsubscribe(chID)

	if err := sendMonitoringSnapshot(ctx, w, a.monitoring); err == nil {
		flusher.Flush()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			payload, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func sendMonitoringSnapshot(ctx context.Context, w http.ResponseWriter, mgr *monitoring.Manager) error {
	if mgr == nil {
		return nil
	}
	summaries, err := mgr.Store().GetSummaries(ctx)
	if err != nil {
		return err
	}
	msg := monitoring.EventMessage{Type: "snapshot", Payload: map[string]any{"sessions": summaries}}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", payload)
	return err
}

func (a *app) monitoringNotifyHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.monitoringEnabled() {
		http.Error(w, "monitoring disabled", http.StatusNotFound)
		return
	}
	if !a.allowMonitorRequest(r) && !a.isAuthenticated(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req struct {
		SessionID string `json:"session_id"`
		Title     string `json:"title"`
		Message   string `json:"message"`
		Level     string `json:"level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.SessionID) == "" || strings.TrimSpace(req.Title) == "" {
		http.Error(w, "session_id and title required", http.StatusBadRequest)
		return
	}
	level := strings.TrimSpace(req.Level)
	if level == "" {
		level = "info"
	}
	status := "running"
	if level == "done" {
		status = "done"
	}
	event := monitoring.Event{
		SessionID: req.SessionID,
		Type:      level,
		Title:     req.Title,
		Message:   req.Message,
		Timestamp: time.Now().UTC(),
	}
	summary := &monitoring.SessionSummary{
		ID:           req.SessionID,
		Name:         req.SessionID,
		Command:      "",
		ProcessID:    0,
		Status:       status,
		Attention:    level,
		LastActivity: time.Now().UTC(),
		CPUPercent:   0,
	}
	if err := a.monitoring.Notify(r.Context(), event, summary); err != nil {
		http.Error(w, "notify failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *app) monitoringEventsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.monitoringEnabled() {
		http.Error(w, "monitoring disabled", http.StatusNotFound)
		return
	}
	if !a.allowMonitorRequest(r) && !a.isAuthenticated(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		http.Error(w, "session_id required", http.StatusBadRequest)
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	events, err := a.monitoring.Store().GetEvents(r.Context(), sessionID, limit)
	if err != nil {
		http.Error(w, "failed to load events", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (a *app) monitoringLogbookHandler(w http.ResponseWriter, r *http.Request) {
	if !a.monitoringEnabled() {
		http.Error(w, "monitoring disabled", http.StatusNotFound)
		return
	}
	if !a.allowMonitorRequest(r) && !a.isAuthenticated(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/monitoring/v1/logbook/")
	id = strings.TrimSpace(id)
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		entries, err := a.monitoring.Store().GetLogbook(r.Context(), id)
		if err != nil {
			http.Error(w, "failed to load logbook", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
	case http.MethodPut:
		var req struct {
			Entries []monitoring.LogbookEntry `json:"entries"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json body", http.StatusBadRequest)
			return
		}
		ctx := r.Context()
		for _, entry := range req.Entries {
			entry.SessionID = id
			if entry.Category == "" {
				continue
			}
			entry.UpdatedAt = time.Now().UTC()
			if err := a.monitoring.Store().UpsertLogbook(ctx, entry); err != nil {
				http.Error(w, "failed to save logbook", http.StatusInternalServerError)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) allowMonitorRequest(r *http.Request) bool {
	if strings.TrimSpace(a.monitorToken) == "" {
		ip := clientIP(r)
		return ip == "127.0.0.1" || ip == "::1" || strings.HasPrefix(ip, "localhost")
	}
	token := strings.TrimSpace(r.Header.Get("X-Webterm-Monitor-Token"))
	return token != "" && token == a.monitorToken
}

func (a *app) isAuthenticated(r *http.Request) bool {
	if a.auth == nil {
		return false
	}
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return false
	}
	_, ok := a.auth.GetSession(cookie.Value)
	return ok
}
