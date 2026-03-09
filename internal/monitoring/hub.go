package monitoring

import "sync"

type Hub struct {
	mu   sync.Mutex
	seq  int
	subs map[int]chan EventMessage
}

type EventMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

func NewHub() *Hub {
	return &Hub{
		subs: map[int]chan EventMessage{},
	}
}

func (h *Hub) Subscribe() (int, <-chan EventMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	ch := make(chan EventMessage, 16)
	h.subs[h.seq] = ch
	return h.seq, ch
}

func (h *Hub) Unsubscribe(id int) {
	h.mu.Lock()
	ch, ok := h.subs[id]
	if ok {
		delete(h.subs, id)
	}
	h.mu.Unlock()
	if ok {
		close(ch)
	}
}

func (h *Hub) Broadcast(msg EventMessage) {
	h.mu.Lock()
	for _, ch := range h.subs {
		select {
		case ch <- msg:
		default:
		}
	}
	h.mu.Unlock()
}
