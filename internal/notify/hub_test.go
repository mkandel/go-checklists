package notify_test

import (
	"testing"
	"time"

	"github.com/mkandel/go-checklists/internal/notify"
)

func TestHub_PublishWakesSubscriber(t *testing.T) {
	h := notify.NewHub()
	ch, unsubscribe := h.Subscribe(1, 100)
	defer unsubscribe()

	h.Publish(1, 100)

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatal("subscriber was not woken")
	}
}

func TestHub_UnsubscribeStopsDelivery(t *testing.T) {
	h := notify.NewHub()
	ch, unsubscribe := h.Subscribe(1, 100)
	unsubscribe()

	h.Publish(1, 100)

	select {
	case <-ch:
		t.Fatal("received a wake-up after unsubscribing")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHub_PublishToUnrelatedKeyDoesNotWake(t *testing.T) {
	h := notify.NewHub()
	ch, unsubscribe := h.Subscribe(1, 100)
	defer unsubscribe()

	h.Publish(1, 999)
	h.Publish(2, 100)

	select {
	case <-ch:
		t.Fatal("received a wake-up for an unrelated (tenantID, userID)")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestHub_PublishIsNonBlockingAgainstFullChannel(t *testing.T) {
	h := notify.NewHub()
	ch, unsubscribe := h.Subscribe(1, 100)
	defer unsubscribe()

	// Channel is buffered size 1 — fill it, then publish again; this must
	// not block even though nothing is draining ch.
	h.Publish(1, 100)

	done := make(chan struct{})
	go func() {
		h.Publish(1, 100)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked against a full subscriber channel")
	}

	// The one buffered slot should still hold a value.
	select {
	case <-ch:
	default:
		t.Fatal("expected a buffered wake-up to be available")
	}
}
