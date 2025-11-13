package devhotload

import (
	"sync"

	"github.com/google/uuid"
)

// Event represents a streamed Dev Hotload event.
type Event struct {
	Type      string      `json:"type"`
	SessionID string      `json:"sessionId"`
	Payload   interface{} `json:"payload,omitempty"`
}

// Notifier fan-outs dev hotload events to SSE subscribers.
type Notifier struct {
	mu    sync.RWMutex
	subs  map[string]chan Event
	limit int
}

// NewNotifier creates a notifier with optional subscription limit (0 = unlimited).
func NewNotifier(limit int) *Notifier {
	return &Notifier{
		subs:  make(map[string]chan Event),
		limit: limit,
	}
}

// Subscribe registers a new listener and returns channel plus cancel func.
func (n *Notifier) Subscribe(buffer int) (id string, ch <-chan Event, cancel func()) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.limit > 0 && len(n.subs) >= n.limit {
		// reuse uuid for meaningful error channel? fallback to drop oldest
	}
	subID := uuid.NewString()
	c := make(chan Event, buffer)
	n.subs[subID] = c
	return subID, c, func() {
		n.unsubscribe(subID)
	}
}

func (n *Notifier) unsubscribe(id string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if ch, ok := n.subs[id]; ok {
		delete(n.subs, id)
		close(ch)
	}
}

// Publish fan-outs event to subscribers.
func (n *Notifier) Publish(event Event) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for _, ch := range n.subs {
		select {
		case ch <- event:
		default:
			// drop if slow consumer
		}
	}
}
