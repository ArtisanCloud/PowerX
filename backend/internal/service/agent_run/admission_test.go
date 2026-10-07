package agent_run

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func TestAdmitReusesRunAcrossRetryAndRejectsKeyReuseForAnotherMessage(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	ctx := context.Background()
	initial := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", SessionID: uuid.NewString(),
		MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: time.Now().Add(time.Hour)}
	first, err := store.Admit(ctx, initial, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	retry := initial
	retry.TraceID = uuid.NewString()
	retry.DeadlineAt = time.Now().Add(2 * time.Hour)
	second, err := store.Admit(ctx, retry, "request-1")
	if err != nil || second.RunID != first.RunID || second.TraceID != first.TraceID || !second.DeadlineAt.Equal(first.DeadlineAt) {
		t.Fatalf("retry did not reuse original run: %+v %+v %v", first, second, err)
	}
	otherMessage := initial
	otherMessage.MessageID = uuid.NewString()
	if _, err := store.Admit(ctx, otherMessage, "request-1"); !errors.Is(err, ErrConflict) {
		t.Fatalf("reused key accepted another message: %v", err)
	}
	otherSession := initial
	otherSession.SessionID = uuid.NewString()
	third, err := store.Admit(ctx, otherSession, "request-1")
	if err != nil || third.RunID == first.RunID {
		t.Fatalf("different session shared run: %+v %v", third, err)
	}
	firstEvents, err := store.Events(ctx, first.TenantUUID, first.Env, first.RunID, 0, 10)
	if err != nil || len(firstEvents) != 1 {
		t.Fatalf("retry duplicated admission event: %+v %v", firstEvents, err)
	}
}

func TestAdmitReturnsExistingRunAfterItsDeadline(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, _ := NewRedisStore(client)
	now := time.Now().UTC()
	store.clock = func() time.Time { return now }
	initial := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", SessionID: uuid.NewString(),
		MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: now.Add(time.Second)}
	first, err := store.Admit(context.Background(), initial, "request-1")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	retry, err := store.Admit(context.Background(), initial, "request-1")
	if err != nil || retry.RunID != first.RunID {
		t.Fatalf("late retry created another run: %+v %v", retry, err)
	}
	if _, err := store.Admit(context.Background(), initial, "new-request"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("new run with expired deadline was accepted: %v", err)
	}
}
