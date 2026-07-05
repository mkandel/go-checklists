// Package notify is an in-process, single-instance broadcaster that wakes
// SSE-connected clients when a notification is created for them. It has no
// history and no delivery queue — subscribers always re-fetch actual state
// (e.g. the unread-count badge) after being woken, so a missed wake is
// harmless. Since this is in-process only, it works only when a single
// checklists-server instance is running; a multi-instance v2 deployment
// would need a shared broker (e.g. Postgres LISTEN/NOTIFY) instead.
package notify

import "sync"

type key struct {
	tenantID int64
	userID   int64
}

// Hub fans out wake-up signals keyed by (tenantID, userID).
type Hub struct {
	mu   sync.Mutex
	subs map[key]map[chan struct{}]struct{}
}

// NewHub returns an empty Hub, ready to use.
func NewHub() *Hub {
	return &Hub{subs: make(map[key]map[chan struct{}]struct{})}
}

// Subscribe registers a new listener for (tenantID, userID) and returns a
// channel that receives a value each time Publish is called for that pair,
// plus an unsubscribe func the caller must invoke (e.g. via defer) once it
// stops listening.
func (h *Hub) Subscribe(tenantID, userID int64) (<-chan struct{}, func()) {
	k := key{tenantID, userID}
	ch := make(chan struct{}, 1)

	h.mu.Lock()
	if h.subs[k] == nil {
		h.subs[k] = make(map[chan struct{}]struct{})
	}
	h.subs[k][ch] = struct{}{}
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		delete(h.subs[k], ch)
		if len(h.subs[k]) == 0 {
			delete(h.subs, k)
		}
		h.mu.Unlock()
	}
	return ch, unsubscribe
}

// Publish wakes every subscriber currently listening for (tenantID, userID).
// Non-blocking: a subscriber whose channel is already full just misses this
// particular wake-up rather than blocking the publisher.
func (h *Hub) Publish(tenantID, userID int64) {
	k := key{tenantID, userID}
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[k] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
