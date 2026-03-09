package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GianlucaP106/gotmux/gotmux"
	"github.com/shirou/gopsutil/v3/process"
)

type Daemon struct {
	serverURL      string
	token          string
	client         *http.Client
	sampleInterval time.Duration
	useTmux        bool
	tmux           *gotmux.Tmux
	seen           map[string]bool
	initialized    bool
}

type sessionInfo struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	LastActive time.Time `json:"last_active"`
}

func NewDaemon(serverURL string, token string, sampleInterval time.Duration) *Daemon {
	if sampleInterval <= 0 {
		sampleInterval = 10 * time.Second
	}
	return &Daemon{
		serverURL:      strings.TrimRight(serverURL, "/"),
		token:          token,
		client:         &http.Client{Timeout: 10 * time.Second},
		sampleInterval: sampleInterval,
		seen:           map[string]bool{},
	}
}

func (d *Daemon) EnableTmux() {
	if tmux, err := gotmux.DefaultTmux(); err == nil {
		d.tmux = tmux
		d.useTmux = true
	}
}

func (d *Daemon) Run(ctx context.Context) error {
	if d.serverURL == "" {
		return errors.New("server URL required")
	}
	if d.useTmux && d.tmux == nil {
		d.EnableTmux()
	}

	_ = d.tick(ctx)

	ticker := time.NewTicker(d.sampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := d.tick(ctx); err != nil {
				continue
			}
		}
	}
}

func (d *Daemon) tick(ctx context.Context) error {
	sessions, err := d.fetchSessions(ctx)
	if err != nil {
		return err
	}
	var summaries []SessionSummary
	var samples []ActivitySample
	var events []Event
	currentSeen := map[string]bool{}
	if d.useTmux && d.tmux != nil {
		for _, sess := range sessions {
			currentSeen[sess.ID] = true
			if d.initialized && !d.seen[sess.ID] {
				events = append(events, Event{
					SessionID: sess.ID,
					Type:      "info",
					Title:     "Session started",
					Message:   "New session detected",
					Timestamp: time.Now().UTC(),
				})
			}
			tmuxName := "webterm-" + sess.ID
			pane, ok := d.firstPaneForSession(tmuxName)
			summary, sample, event := d.buildSummary(sess, pane, ok)
			summaries = append(summaries, summary)
			samples = append(samples, sample)
			if event != nil {
				events = append(events, *event)
			}
		}
	} else {
		for _, sess := range sessions {
			currentSeen[sess.ID] = true
			if d.initialized && !d.seen[sess.ID] {
				events = append(events, Event{
					SessionID: sess.ID,
					Type:      "info",
					Title:     "Session started",
					Message:   "New session detected",
					Timestamp: time.Now().UTC(),
				})
			}
			summary, sample := d.buildBasicSummary(sess)
			summaries = append(summaries, summary)
			samples = append(samples, sample)
		}
	}
	d.seen = currentSeen
	d.initialized = true

	return d.ingest(ctx, summaries, samples, events)
}

func (d *Daemon) firstPaneForSession(name string) (*gotmux.Pane, bool) {
	if d.tmux == nil {
		return nil, false
	}
	sess, err := d.tmux.GetSessionByName(name)
	if err != nil || sess == nil {
		return nil, false
	}
	panes, err := sess.ListPanes()
	if err != nil || len(panes) == 0 {
		return nil, false
	}
	return panes[0], true
}

func (d *Daemon) buildSummary(sess sessionInfo, pane *gotmux.Pane, ok bool) (SessionSummary, ActivitySample, *Event) {
	now := time.Now().UTC()
	status := "running"
	attention := "low"
	command := ""
	pid := int32(0)
	activityScore := 0.0
	cpuPercent := 0.0
	memoryBytes := uint64(0)
	if ok {
		command = pane.CurrentCommand
		pid = pane.Pid
		if pane.Dead {
			status = "done"
			attention = "done"
		} else if pane.UnseenChanges {
			attention = "high"
			activityScore = 1
		}
		if pid > 0 {
			if proc, err := process.NewProcess(pid); err == nil {
				if val, err := proc.CPUPercent(); err == nil {
					cpuPercent = val
				}
				if mem, err := proc.MemoryInfo(); err == nil {
					memoryBytes = mem.RSS
				}
			}
		}
	}
	if !sess.LastActive.IsZero() {
		if now.Sub(sess.LastActive) > 2*time.Minute {
			status = "idle"
			if attention == "low" {
				attention = "idle"
			}
		} else if activityScore == 0 {
			activityScore = 0.5
		}
	}
	var event *Event
	if ok && pane.UnseenChanges {
		if captured, err := pane.Capture(); err == nil {
			lower := strings.ToLower(captured)
			if strings.Contains(lower, "error") || strings.Contains(lower, "failed") || strings.Contains(lower, "panic") {
				attention = "error"
				status = "error"
				event = &Event{
					SessionID: sess.ID,
					Type:      "error",
					Title:     "Error detected",
					Message:   "Detected error output in session",
					Timestamp: now,
				}
			}
			if strings.Contains(lower, "waiting for input") || strings.Contains(lower, "permission") {
				attention = "wait"
			}
		}
	}

	summary := SessionSummary{
		ID:           sess.ID,
		Name:         sess.Name,
		Command:      command,
		ProcessID:    int(pid),
		Status:       status,
		Attention:    attention,
		LastActivity: sess.LastActive,
		CPUPercent:   cpuPercent,
		MemoryBytes:  memoryBytes,
		GPUUtil:      -1,
	}
	sample := ActivitySample{
		SessionID:     sess.ID,
		Timestamp:     now,
		ActivityScore: activityScore,
		CPUPercent:    cpuPercent,
	}
	return summary, sample, event
}

func (d *Daemon) buildBasicSummary(sess sessionInfo) (SessionSummary, ActivitySample) {
	now := time.Now().UTC()
	attention := "low"
	status := "running"
	activityScore := 0.0
	if !sess.LastActive.IsZero() {
		age := now.Sub(sess.LastActive)
		if age > 2*time.Minute {
			attention = "idle"
			status = "idle"
		} else {
			activityScore = 0.5
		}
	}
	return SessionSummary{
			ID:           sess.ID,
			Name:         sess.Name,
			Command:      "",
			ProcessID:    0,
			Status:       status,
			Attention:    attention,
			LastActivity: sess.LastActive,
			CPUPercent:   0,
			MemoryBytes:  0,
			GPUUtil:      -1,
		}, ActivitySample{
			SessionID:     sess.ID,
			Timestamp:     now,
			ActivityScore: activityScore,
			CPUPercent:    0,
		}
}

func (d *Daemon) fetchSessions(ctx context.Context) ([]sessionInfo, error) {
	url := d.serverURL + "/api/monitoring/v1/sessions"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(d.token) != "" {
		req.Header.Set("X-Webterm-Monitor-Token", d.token)
	}
	res, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, errors.New("failed to fetch sessions")
	}
	var payload struct {
		Sessions []sessionInfo `json:"sessions"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Sessions, nil
}

func (d *Daemon) ingest(ctx context.Context, summaries []SessionSummary, samples []ActivitySample, events []Event) error {
	url := d.serverURL + "/api/monitoring/v1/ingest"
	body, err := json.Marshal(map[string]any{
		"sessions": summaries,
		"samples":  samples,
		"events":   events,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(d.token) != "" {
		req.Header.Set("X-Webterm-Monitor-Token", d.token)
	}
	res, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil
	}
	return nil
}
