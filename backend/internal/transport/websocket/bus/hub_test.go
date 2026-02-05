package bus

import (
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

func newTestClient(id, tenant string) *Client {
	return &Client{
		ID:         id,
		TenantUUID: tenant,
		send:       make(chan dto.WSBusEnvelope, 1),
		topics:     make(map[string]struct{}),
	}
}

func TestHubPublishTenantIsolation(t *testing.T) {
	hub := NewHub()
	c1 := newTestClient("c1", "tenant-a")
	c2 := newTestClient("c2", "tenant-b")

	hub.Register(c1)
	hub.Register(c2)
	hub.Subscribe(c1, "topic")
	hub.Subscribe(c2, "topic")

	hub.Publish("tenant-a", "topic", map[string]any{"ok": true}, "trace-1")

	select {
	case env := <-c1.send:
		if env.Type != dto.WSBusTypeEvent || env.Topic != "topic" {
			t.Fatalf("unexpected envelope: %#v", env)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected message for tenant-a")
	}

	select {
	case env := <-c2.send:
		t.Fatalf("unexpected message for tenant-b: %#v", env)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestHubUnsubscribeStopsDelivery(t *testing.T) {
	hub := NewHub()
	client := newTestClient("c1", "tenant-a")

	hub.Register(client)
	hub.Subscribe(client, "topic")
	hub.Unsubscribe(client, "topic")

	hub.Publish("tenant-a", "topic", map[string]any{"ok": true}, "trace-2")

	select {
	case env := <-client.send:
		t.Fatalf("unexpected message after unsubscribe: %#v", env)
	case <-time.After(100 * time.Millisecond):
	}
}
