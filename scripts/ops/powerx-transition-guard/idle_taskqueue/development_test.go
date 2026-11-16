package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDevelopmentRetentionExceptionCannotChangeProduction(t *testing.T) {
	now := time.Now()
	commit := strings.Repeat("a", 40)
	for _, c := range []struct {
		db     string
		hours  int
		commit string
		age    time.Duration
		ok     bool
	}{
		{"powerx_dev", 24, commit, 25 * time.Hour, true},
		{"powerx_dev", 24, commit, 23 * time.Hour, false},
		{"powerx_pro", 24, commit, 8 * 24 * time.Hour, false},
		{"powerx_dev", 24, "", 25 * time.Hour, false},
		{"powerx_dev", 1, commit, 25 * time.Hour, false},
		{"powerx_pro", 0, "", 8 * 24 * time.Hour, true},
		{"powerx_pro", 0, "", 25 * time.Hour, false},
		{"powerx_dev", 0, "", 25 * time.Hour, false},
	} {
		if got := validRetentionCutoff(c.db, c.hours, c.commit, now.Add(-c.age), now); (got == nil) != c.ok {
			t.Fatalf("db=%s hours=%d age=%s got=%v", c.db, c.hours, c.age, got)
		}
	}
}
func TestDevelopmentRuntimeMustRemainPinned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	commit := strings.Repeat("a", 40)
	if err := os.WriteFile(path, []byte(`{"source_commit":"`+commit+`"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkDevelopmentCommit(path, commit); err != nil {
		t.Fatal(err)
	}
	if err := checkDevelopmentCommit(path, strings.Repeat("b", 40)); err == nil {
		t.Fatal("runtime change accepted")
	}
}

func TestDevelopmentTwentyFourHourScopePreservesRecentAndFailure(t *testing.T) {
	c, _ := fixture(t)
	ctx := context.Background()
	for _, sql := range []string{
		"UPDATE " + traces + " SET created_at=now()-interval '2 days',updated_at=now()-interval '2 days' WHERE id IN (1,2,3)",
		"UPDATE " + events + " SET created_at=now()-interval '2 days',updated_at=now()-interval '2 days' WHERE id IN (1,2,3)",
		"UPDATE " + traces + " SET created_at=now()-interval '23 hours',updated_at=now()-interval '23 hours' WHERE id=2",
	} {
		if _, err := c.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	beforeT, beforeE := snapshot(t, c, traces), snapshot(t, c, events)
	r := newRunner(t, c, time.Now().Add(-24*time.Hour))
	r.batch = 1000
	n, _, err := r.one(ctx, 0, 3, true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("pairs=%d", n)
	}
	assertUnchanged(t, beforeT, snapshot(t, c, traces), map[int64]bool{1: true})
	assertUnchanged(t, beforeE, snapshot(t, c, events), map[int64]bool{1: true})
}

func TestDevelopmentUnroutedReceiptCannotDeleteDeliveredOrRetried(t *testing.T) {
	for _, tc := range []struct {
		name, mutation string
		eligible       bool
	}{
		{"zero_subscriber_ack", "", true},
		{"delivered", "UPDATE " + events + " SET status='published' WHERE id=1", false},
		{"retried", "UPDATE " + events + " SET attempts=2 WHERE id=1", false},
		{"unknown_receipt", "UPDATE " + events + " SET delivery_receipt='unknown' WHERE id=1", false},
		{"unknown_class", "UPDATE " + events + " SET record_class='legacy_unclassified' WHERE id=1", false},
		{"unknown_terminal", "UPDATE " + events + " SET terminal_at=NULL WHERE id=1", false},
		{"error", "UPDATE " + events + " SET last_error='failure' WHERE id=1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, cut := fixture(t)
			ctx := context.Background()
			_, err := c.Exec(ctx, "UPDATE "+events+" SET record_class='capability_invocation_notification',status='unrouted',attempts=1,publish_time=created_at,terminal_at=created_at,delivery_receipt='11111111-1111-4111-8111-111111111111' WHERE id=1")
			if err != nil {
				t.Fatal(err)
			}
			if tc.mutation != "" {
				if _, err = c.Exec(ctx, tc.mutation); err != nil {
					t.Fatal(err)
				}
			}
			beforeT, beforeE := snapshot(t, c, traces), snapshot(t, c, events)
			r := newRunner(t, c, cut)
			n, _, err := r.one(ctx, 0, 1, false)
			if err != nil || n != 0 {
				t.Fatalf("default seven-day contract expanded: %d %v", n, err)
			}
			r.devUnroutedReceipt = true
			n, _, err = r.one(ctx, 0, 1, true)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			removed := map[int64]bool{}
			if tc.eligible {
				want = 1
				removed[1] = true
			}
			if n != want {
				t.Fatalf("pairs=%d", n)
			}
			assertUnchanged(t, beforeT, snapshot(t, c, traces), removed)
			assertUnchanged(t, beforeE, snapshot(t, c, events), removed)
		})
	}
}

func TestDevelopmentMaintenanceStopsWhenCoreExecutionControlIsAdopted(t *testing.T) {
	c, cut := fixture(t)
	ctx := context.Background()
	scope := strings.Repeat("a", 64)
	_, err := c.Exec(ctx, "DROP TABLE IF EXISTS public.ops_storage_control_configurations; CREATE TABLE public.ops_storage_control_configurations(tenant_id bigint,scope_id text,domain text)")
	if err != nil {
		t.Fatal(err)
	}
	r := newRunner(t, c, cut)
	r.devScope = scope
	n, _, err := r.one(ctx, 0, 1, false)
	if err != nil || n != 1 {
		t.Fatalf("preview %d %v", n, err)
	}
	_, err = c.Exec(ctx, "INSERT INTO public.ops_storage_control_configurations VALUES(0,$1,'execution-control')", scope)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, c, traces)
	_, _, err = r.one(ctx, 0, 1, true)
	if err == nil {
		t.Fatal("adopted Core fence ignored")
	}
	assertUnchanged(t, before, snapshot(t, c, traces), map[int64]bool{})
}
