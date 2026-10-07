// Package bus fans out realtime events (WsEvent payloads) to connected clients.
package bus

import (
	"encoding/json"
	"log/slog"
	"sync"
)

type Bus struct {
	mu   sync.Mutex
	subs map[*Sub]struct{}
}

// Sub is a subscription. C receives JSON-encoded events and is closed when the subscription
// ends (on Close, or when the subscriber falls too far behind).
type Sub struct {
	C   chan []byte
	bus *Bus
}

func New() *Bus { return &Bus{subs: map[*Sub]struct{}{}} }

func (b *Bus) Subscribe() *Sub {
	s := &Sub{C: make(chan []byte, 256), bus: b}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()
	return s
}

func (s *Sub) Close() {
	s.bus.mu.Lock()
	defer s.bus.mu.Unlock()
	if _, ok := s.bus.subs[s]; ok {
		delete(s.bus.subs, s)
		close(s.C)
	}
}

// Publish sends an event to every subscriber. Subscribers whose buffer is full are dropped;
// clients resync on reconnect.
func (b *Bus) Publish(event any) {
	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("bus: marshal event", "err", err)
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		select {
		case s.C <- data:
		default:
			delete(b.subs, s)
			close(s.C)
		}
	}
}
