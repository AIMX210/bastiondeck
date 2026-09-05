package sshlite

import (
	"context"
	"sync"
	"time"

	"bastiondeck/internal/connector"
)

// Pool caches established SSH clients keyed by host id and reaps idle ones.
type Pool struct {
	dialer *Dialer
	idle   time.Duration

	mu      sync.Mutex
	entries map[string]*poolEntry
}

type poolEntry struct {
	client   *Client
	lastUsed time.Time
	timer    *time.Timer
}

// NewPool constructs a pool over a dialer.
func NewPool(d *Dialer, idle time.Duration) *Pool {
	if idle <= 0 {
		idle = 5 * time.Minute
	}
	return &Pool{dialer: d, idle: idle, entries: map[string]*poolEntry{}}
}

// Connect returns a cached client or dials a new one.
func (p *Pool) Connect(ctx context.Context, hostID string) (connector.Client, error) {
	p.mu.Lock()
	if e, ok := p.entries[hostID]; ok {
		select {
		case <-e.client.Done():
			p.removeLocked(hostID)
		default:
			e.lastUsed = time.Now()
			c := e.client
			p.mu.Unlock()
			return c, nil
		}
	}
	p.mu.Unlock()

	c, err := p.dialer.Connect(ctx, hostID)
	if err != nil {
		return nil, err
	}
	sc := c.(*Client)
	p.mu.Lock()
	// Another goroutine may have connected concurrently; prefer the first.
	if old, ok := p.entries[hostID]; ok {
		p.mu.Unlock()
		_ = sc.Close()
		return old.client, nil
	}
	e := &poolEntry{client: sc, lastUsed: time.Now()}
	e.timer = time.AfterFunc(p.idle, func() { p.evict(hostID) })
	p.entries[hostID] = e
	p.mu.Unlock()
	return sc, nil
}

func (p *Pool) evict(hostID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.removeLocked(hostID)
}

func (p *Pool) removeLocked(hostID string) {
	if e, ok := p.entries[hostID]; ok {
		if e.timer != nil {
			e.timer.Stop()
		}
		_ = e.client.Close()
		delete(p.entries, hostID)
	}
}

// Invalidate closes and forgets a host's connection (e.g. key change).
func (p *Pool) Invalidate(hostID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.removeLocked(hostID)
}

// CloseAll evicts every pooled connection.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id := range p.entries {
		p.removeLocked(id)
	}
}

// Stats reports current pool size (for doctor/metrics).
func (p *Pool) Stats() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return map[string]int{"pooled": len(p.entries)}
}
