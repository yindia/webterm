package monitoring

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func OpenStore(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("monitoring db path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA synchronous = NORMAL"); err != nil {
		_ = db.Close()
		return nil, err
	}
	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			command TEXT NOT NULL,
			process_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			attention TEXT NOT NULL,
			last_activity INTEGER NOT NULL,
			cpu_percent REAL NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS samples (
			session_id TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			activity_score REAL NOT NULL,
			cpu_percent REAL NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_samples_session_time ON samples(session_id, timestamp);`,
		`CREATE TABLE IF NOT EXISTS events (
			session_id TEXT NOT NULL,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			message TEXT NOT NULL,
			timestamp INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_events_session_time ON events(session_id, timestamp);`,
		`CREATE TABLE IF NOT EXISTS logbook (
			session_id TEXT NOT NULL,
			category TEXT NOT NULL,
			note TEXT NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY(session_id, category)
		);`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpsertSession(ctx context.Context, summary SessionSummary) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO sessions (id, name, command, process_id, status, attention, last_activity, cpu_percent, memory_bytes, gpu_util)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		 name = excluded.name,
		 command = excluded.command,
		 process_id = excluded.process_id,
		 status = excluded.status,
		 attention = excluded.attention,
		 last_activity = excluded.last_activity,
		 cpu_percent = excluded.cpu_percent,
		 memory_bytes = excluded.memory_bytes,
		 gpu_util = excluded.gpu_util`,
		summary.ID,
		summary.Name,
		summary.Command,
		summary.ProcessID,
		summary.Status,
		summary.Attention,
		summary.LastActivity.Unix(),
		summary.CPUPercent,
		summary.MemoryBytes,
		summary.GPUUtil,
	)
	return err
}

func (s *Store) InsertSample(ctx context.Context, sample ActivitySample) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO samples (session_id, timestamp, activity_score, cpu_percent) VALUES (?, ?, ?, ?)`,
		sample.SessionID,
		sample.Timestamp.Unix(),
		sample.ActivityScore,
		sample.CPUPercent,
	)
	return err
}

func (s *Store) InsertEvent(ctx context.Context, event Event) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO events (session_id, type, title, message, timestamp) VALUES (?, ?, ?, ?, ?)`,
		event.SessionID,
		event.Type,
		event.Title,
		event.Message,
		event.Timestamp.Unix(),
	)
	return err
}

func (s *Store) GetSummaries(ctx context.Context) ([]SessionSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, command, process_id, status, attention, last_activity, cpu_percent FROM sessions ORDER BY last_activity DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SessionSummary
	for rows.Next() {
		var summary SessionSummary
		var lastActivity int64
		if err := rows.Scan(&summary.ID, &summary.Name, &summary.Command, &summary.ProcessID, &summary.Status, &summary.Attention, &lastActivity, &summary.CPUPercent); err != nil {
			return nil, err
		}
		summary.LastActivity = time.Unix(lastActivity, 0).UTC()
		out = append(out, summary)
	}
	return out, rows.Err()
}

func (s *Store) GetActivitySeries(ctx context.Context, sessionID string, limit int) ([]ActivityPoint, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT timestamp, activity_score FROM samples WHERE session_id = ? ORDER BY timestamp DESC LIMIT ?`,
		sessionID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := make([]ActivityPoint, 0, limit)
	for rows.Next() {
		var ts int64
		var score float64
		if err := rows.Scan(&ts, &score); err != nil {
			return nil, err
		}
		points = append(points, ActivityPoint{Timestamp: time.Unix(ts, 0).UTC(), Score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
		points[i], points[j] = points[j], points[i]
	}
	return points, nil
}

func (s *Store) GetEvents(ctx context.Context, sessionID string, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, type, title, message, timestamp FROM events WHERE session_id = ? ORDER BY timestamp DESC LIMIT ?`,
		sessionID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var event Event
		var ts int64
		if err := rows.Scan(&event.SessionID, &event.Type, &event.Title, &event.Message, &ts); err != nil {
			return nil, err
		}
		event.Timestamp = time.Unix(ts, 0).UTC()
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *Store) UpsertLogbook(ctx context.Context, entry LogbookEntry) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO logbook (session_id, category, note, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(session_id, category) DO UPDATE SET
		 note = excluded.note,
		 updated_at = excluded.updated_at`,
		entry.SessionID,
		entry.Category,
		entry.Note,
		entry.UpdatedAt.Unix(),
	)
	return err
}

func (s *Store) GetLogbook(ctx context.Context, sessionID string) ([]LogbookEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id, category, note, updated_at FROM logbook WHERE session_id = ? ORDER BY category`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LogbookEntry
	for rows.Next() {
		var entry LogbookEntry
		var ts int64
		if err := rows.Scan(&entry.SessionID, &entry.Category, &entry.Note, &ts); err != nil {
			return nil, err
		}
		entry.UpdatedAt = time.Unix(ts, 0).UTC()
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (s *Store) Cleanup(ctx context.Context, retention time.Duration) error {
	if retention <= 0 {
		return nil
	}
	cutoff := time.Now().Add(-retention).Unix()
	_, err := s.db.ExecContext(ctx, `DELETE FROM samples WHERE timestamp < ?`, cutoff)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM events WHERE timestamp < ?`, cutoff)
	return err
}
