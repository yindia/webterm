package monitoring

import "time"

type SessionSummary struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Command      string    `json:"command"`
	ProcessID    int       `json:"process_id"`
	Status       string    `json:"status"`
	Attention    string    `json:"attention"`
	LastActivity time.Time `json:"last_activity"`
	CPUPercent   float64   `json:"cpu_percent"`
	MemoryBytes  uint64    `json:"memory_bytes"`
	GPUUtil      float64   `json:"gpu_util"`
}

type ActivitySample struct {
	SessionID     string    `json:"session_id"`
	Timestamp     time.Time `json:"timestamp"`
	ActivityScore float64   `json:"activity_score"`
	CPUPercent    float64   `json:"cpu_percent"`
}

type ActivityPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Score     float64   `json:"score"`
}

type Event struct {
	SessionID string    `json:"session_id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type LogbookEntry struct {
	SessionID string    `json:"session_id"`
	Category  string    `json:"category"`
	Note      string    `json:"note"`
	UpdatedAt time.Time `json:"updated_at"`
}
