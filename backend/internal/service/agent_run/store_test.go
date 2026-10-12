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

func TestRunStoreAtomicTransitionAndReplay(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := NewRedisStore(client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	initial := Snapshot{
		TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(),
		SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(),
		Status: "accepted", DeadlineAt: time.Now().Add(time.Hour),
	}
	created, err := store.Create(ctx, initial)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := store.Create(ctx, initial)
	if err != nil || duplicate.Version != created.Version {
		t.Fatalf("idempotent create: snapshot=%+v err=%v", duplicate, err)
	}
	mismatchedTrace := initial
	mismatchedTrace.TraceID = uuid.NewString()
	if _, err := store.Create(ctx, mismatchedTrace); !errors.Is(err, ErrConflict) {
		t.Fatalf("same run id accepted a different trace: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Transition(ctx, initial, created.Version, "running", "agent_run.task_status")
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected transition error: %v", err)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("want one winner and one stale writer, got success=%d conflicts=%d", success, conflicts)
	}
	current, err := store.Get(ctx, initial.TenantUUID, initial.Env, initial.RunID)
	if err != nil || current.Version != 2 || current.EventSeq != 2 || current.Status != "running" {
		t.Fatalf("invalid stored state: %+v err=%v", current, err)
	}
	events, err := store.Events(ctx, initial.TenantUUID, initial.Env, initial.RunID, 0, 10)
	if err != nil || len(events) != 2 || events[0].Seq != 1 || events[1].Seq != 2 {
		t.Fatalf("invalid authoritative event history: %+v err=%v", events, err)
	}
	replayed, err := store.Events(ctx, initial.TenantUUID, initial.Env, initial.RunID, 1, 10)
	if err != nil || len(replayed) != 1 || replayed[0].Seq != 2 {
		t.Fatalf("invalid event cursor replay: %+v err=%v", replayed, err)
	}
	_, eventsKey := runKeys(initial)
	if err := client.XDel(ctx, eventsKey, "1-0").Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Events(ctx, initial.TenantUUID, initial.Env, initial.RunID, 0, 10); !errors.Is(err, ErrEventCursorExpired) {
		t.Fatalf("trimmed event history did not require snapshot recovery: %v", err)
	}
}

func TestRecoverPendingAdmissionAfterDeadline(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = client.Close() })
	store, err := NewRedisStore(client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	initial := Snapshot{TenantUUID: uuid.NewString(), Env: "dev", RunID: uuid.NewString(), SessionID: uuid.NewString(), MessageID: uuid.NewString(), TraceID: uuid.NewString(), Status: "accepted", DeadlineAt: time.Now().Add(-time.Minute)}
	if _, err := store.Create(ctx, initial); !errors.Is(err, ErrInvalid) {
		t.Fatalf("fresh expired admission: %v", err)
	}
	run, err := store.RecoverAdmission(ctx, initial)
	if err != nil || run.Status != "failed" || run.PlanRevision != 0 {
		t.Fatalf("expired recovery: %+v %v", run, err)
	}
	events, err := store.Events(ctx, run.TenantUUID, run.Env, run.RunID, 0, 10)
	if err != nil || len(events) != 1 || events[0].ReasonCode != "run.deadline" {
		t.Fatalf("deadline receipt: %+v %v", events, err)
	}
	replay, err := store.RecoverAdmission(ctx, initial)
	if err != nil || replay.Version != run.Version {
		t.Fatalf("recovery replay: %+v %v", replay, err)
	}
}
