package state

import "sync"

// Event is one connector state change delivered to SSE subscribers.
type Event struct {
	Type      string         `json:"type"`
	Connector ConnectorState `json:"connector"`
}

// Hub fans out connector updates to all SSE subscribers. Each subscriber gets
// its own buffered channel: a slow browser drops old updates instead of
// blocking the device-reporting goroutine.
type Hub struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

func NewHub() *Hub { return &Hub{subs: make(map[chan Event]struct{})} }

// Subscribe registers a new listener. Callers must Unsubscribe when done.
func (h *Hub) Subscribe() chan Event {
	ch := make(chan Event, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) Unsubscribe(ch chan Event) {
	h.mu.Lock()
	if _, ok := h.subs[ch]; ok {
		delete(h.subs, ch)
		close(ch)
	}
	h.mu.Unlock()
}

// Publish delivers an event to every subscriber without blocking. Full
// buffers drop the oldest event to keep each client's view moving forward.
func (h *Hub) Publish(e Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- e:
		default:
			select {
			case <-ch: // drop stale
			default:
			}
			select {
			case ch <- e:
			default:
			}
		}
	}
}

func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}
