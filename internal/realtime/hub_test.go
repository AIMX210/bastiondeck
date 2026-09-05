package realtime

import (
	"testing"
	"time"
)

func TestHubFanout(t *testing.T) {
	h := NewHub()
	_, ch1, off1 := h.Subscribe(4)
	defer off1()
	_, ch2, off2 := h.Subscribe(4)
	defer off2()

	h.Publish("run_update", map[string]string{"runId": "r1"})
	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Type != "run_update" {
				t.Fatalf("sub %d type %q", i, ev.Type)
			}
		case <-time.After(time.Second):
			t.Fatalf("sub %d did not receive", i)
		}
	}
	if h.SubscriberCount() != 2 {
		t.Fatalf("count = %d", h.SubscriberCount())
	}
}

func TestHubUnsubscribe(t *testing.T) {
	h := NewHub()
	_, ch, off := h.Subscribe(2)
	off()
	if h.SubscriberCount() != 0 {
		t.Fatal("unsubscribe did not remove subscriber")
	}
	h.Publish("x", nil)
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("event delivered after unsubscribe")
		}
	default:
	}
}

func TestHubDropsSlowSubscriber(t *testing.T) {
	h := NewHub()
	_, ch, off := h.Subscribe(1)
	defer off()
	// Buffer is 1; publish three events without reading.
	h.Publish("a", 1)
	h.Publish("b", 2)
	h.Publish("c", 3)
	// Drain the single buffered event; extras must have been dropped (no block,
	// no panic) and Dropped counter advanced.
	<-ch
	if h.Dropped() < 2 {
		t.Fatalf("expected >=2 drops, got %d", h.Dropped())
	}
}

func TestHubSafeWithoutSubscribers(t *testing.T) {
	h := NewHub()
	// Publishing to an empty hub must not panic or block.
	done := make(chan struct{})
	go func() { h.Publish("noop", nil); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publish blocked with no subscribers")
	}
}
