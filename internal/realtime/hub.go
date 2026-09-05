// Package realtime is the in-process publish/subscribe bus that powers SSE
// and WebSocket event streams (run progress, host status, audit feed). A slow
// consumer is dropped rather than allowed to block publishers.
package realtime

import (
	"sync"
	"sync/atomic"

	"bastiondeck/internal/store"
)

// Event is one broadcast message.
type Event struct {
	Type string `json:"type"`
	At   string `json:"at"`
	Data any    `json:"data"`
}

// Hub is a simple fan-out broker.
type Hub struct {
	mu      sync.RWMutex
	next    int64
	subs    map[int64]subscriber
	dropCnt atomic.Int64
}

type subscriber struct {
	ch   chan Event
	done chan struct{}
}

// NewHub constructs an empty hub.
func NewHub() *Hub { return &Hub{subs: map[int64]subscriber{}} }

// Subscribe returns an id and a buffered event channel.
func (h *Hub) Subscribe(buffer int) (int64, <-chan Event, func()) {
	if buffer <= 0 {
		buffer = 64
	}
	id := atomic.AddInt64(&h.next, 1)
	s := subscriber{ch: make(chan Event, buffer), done: make(chan struct{})}
	h.mu.Lock()
	h.subs[id] = s
	h.mu.Unlock()
	unsub := func() {
		h.mu.Lock()
		if cur, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(cur.done)
		}
		h.mu.Unlock()
	}
	return id, s.ch, unsub
}

// Publish fans out without blocking; a full subscriber buffer drops the
// event and counts it.
func (h *Hub) Publish(t string, data any) {
	ev := Event{Type: t, At: store.Now(), Data: data}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, s := range h.subs {
		select {
		case s.ch <- ev:
		default:
			h.dropCnt.Add(1)
		}
	}
}

// Dropped returns the number of dropped events (doctor/metrics).
func (h *Hub) Dropped() int64 { return h.dropCnt.Load() }

// SubscriberCount reports current fan-out width.
func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}
