package monitoring

import (
	"context"
	"time"
)

type Manager struct {
	store     *Store
	hub       *Hub
	retention time.Duration
}

func NewManager(store *Store, retention time.Duration) *Manager {
	return &Manager{
		store:     store,
		hub:       NewHub(),
		retention: retention,
	}
}

func (m *Manager) Store() *Store {
	return m.store
}

func (m *Manager) Hub() *Hub {
	return m.hub
}

func (m *Manager) Ingest(ctx context.Context, sessions []SessionSummary, samples []ActivitySample, events []Event) error {
	for _, session := range sessions {
		if err := m.store.UpsertSession(ctx, session); err != nil {
			return err
		}
	}
	for _, sample := range samples {
		if err := m.store.InsertSample(ctx, sample); err != nil {
			return err
		}
	}
	for _, event := range events {
		if err := m.store.InsertEvent(ctx, event); err != nil {
			return err
		}
	}
	if m.retention > 0 {
		_ = m.store.Cleanup(ctx, m.retention)
	}
	if len(sessions) > 0 || len(events) > 0 {
		m.hub.Broadcast(EventMessage{Type: "update", Payload: map[string]any{"sessions": sessions}})
	}
	return nil
}

func (m *Manager) Notify(ctx context.Context, event Event, summary *SessionSummary) error {
	if summary != nil {
		if err := m.store.UpsertSession(ctx, *summary); err != nil {
			return err
		}
	}
	if err := m.store.InsertEvent(ctx, event); err != nil {
		return err
	}
	m.hub.Broadcast(EventMessage{Type: "event", Payload: event})
	return nil
}
