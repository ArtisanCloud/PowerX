package agent_run

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type memoryReportObjects struct {
	mu      sync.Mutex
	items   map[string][]byte
	failPut bool
}

func (m *memoryReportObjects) Put(_ context.Context, key string, body []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failPut {
		return errors.New("object storage unavailable")
	}
	m.items[key] = append([]byte(nil), body...)
	return nil
}

func (m *memoryReportObjects) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	body, ok := m.items[key]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), body...), nil
}

func TestArchiveRequiresReadableVerifiedObject(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	ctx := context.Background()
	identity := Snapshot{
		TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: time.Now().Add(time.Hour),
	}
	run, err := store.Create(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	run, err = store.Transition(ctx, identity, run.Version, "completed", "agent_run.ended")
	if err != nil {
		t.Fatal(err)
	}
	objects := &memoryReportObjects{items: make(map[string][]byte), failPut: true}
	if _, err := store.ArchiveRun(ctx, identity, objects); err == nil {
		t.Fatal("unavailable object store was ignored")
	}
	current, err := store.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
	if err != nil || current.ArchiveKey != "" {
		t.Fatalf("failed archive was marked complete: %+v %v", current, err)
	}
	events, err := store.Events(ctx, identity.TenantUUID, identity.Env, identity.RunID, 0, 10)
	if err != nil || len(events) != 2 {
		t.Fatalf("failed archive discarded Redis events: %+v %v", events, err)
	}
	objects.failPut = false
	key, err := store.ArchiveRun(ctx, identity, objects)
	if err != nil || key == "" {
		t.Fatalf("archive failed: %s %v", key, err)
	}
	current, err = store.Get(ctx, identity.TenantUUID, identity.Env, identity.RunID)
	if err != nil || current.ArchiveKey != key || current.ArchivedAt.IsZero() {
		t.Fatalf("archive locator missing: %+v %v", current, err)
	}
	report, err := ReadRunReport(ctx, identity, objects)
	if err != nil || report.Snapshot.RunID != identity.RunID || len(report.Events) != 2 {
		t.Fatalf("archived report invalid: %+v %v", report, err)
	}
	objects.mu.Lock()
	objects.items[key][len(objects.items[key])-2] ^= 1
	objects.mu.Unlock()
	if _, err := ReadRunReport(ctx, identity, objects); err == nil {
		t.Fatal("corrupted report passed checksum validation")
	}
}
