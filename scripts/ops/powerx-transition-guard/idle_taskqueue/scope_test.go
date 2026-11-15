package main

import (
	"context"
	"testing"
)

func TestIdleScopeRequiresTypedEmptyResultAndMatchingNotification(t *testing.T) {
	cases := []struct {
		name, mutation string
		eligible       bool
	}{
		{"empty_legacy", "", true},
		{"empty_summary", `UPDATE ` + traces + ` SET response_payload='{"contract_version":"taskqueue.v2","messages":{"_omitted":"content","size":0}}' WHERE id=1`, true},
		{"classified_unrouted", `UPDATE ` + traces + ` SET record_class='capability_invocation_audit',completed_at=created_at WHERE id=1; UPDATE ` + events + ` SET record_class='capability_invocation_notification',status='unrouted',terminal_at=created_at WHERE id=1`, true},
		{"real_message", `UPDATE ` + traces + ` SET response_payload='{"contract_version":"taskqueue.v2","messages":[{"id":"real"}]}' WHERE id=1`, false},
		{"nonempty_summary", `UPDATE ` + traces + ` SET response_payload='{"contract_version":"taskqueue.v2","messages":{"_omitted":"content","size":1}}' WHERE id=1`, false},
		{"missing_response_contract", `UPDATE ` + traces + ` SET response_payload='{"messages":[]}' WHERE id=1`, false},
		{"missing_request_contract", `UPDATE ` + traces + ` SET request_payload=request_payload #- '{body,contract_version}' WHERE id=1`, false},
		{"ack_is_business_operation", `UPDATE ` + traces + ` SET request_payload=jsonb_set(request_payload,'{body,operation}','"ack"') WHERE id=1`, false},
		{"request_foreign_tenant", `UPDATE ` + traces + ` SET request_payload=jsonb_set(request_payload,'{body,tenant_key}','"foreign"') WHERE id=1`, false},
		{"fallback", `UPDATE ` + traces + ` SET fallback_used=true WHERE id=1`, false},
		{"error", `UPDATE ` + traces + ` SET error_summary='failure' WHERE id=1`, false},
		{"recent_completion", `UPDATE ` + traces + ` SET completed_at=now() WHERE id=1`, false},
		{"unknown_class", `UPDATE ` + traces + ` SET record_class='business' WHERE id=1`, false},
		{"attempted_notification", `UPDATE ` + events + ` SET attempts=1 WHERE id=1`, false},
		{"delivery_receipt", `UPDATE ` + events + ` SET delivery_receipt='receipt' WHERE id=1`, false},
		{"recent_unrouted", `UPDATE ` + events + ` SET status='unrouted',terminal_at=now() WHERE id=1`, false},
		{"notification_failed", `UPDATE ` + events + ` SET status='failed',terminal_at=created_at WHERE id=1`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, cutoff := fixture(t)
			if tc.mutation != "" {
				if _, err := c.Exec(context.Background(), tc.mutation); err != nil {
					t.Fatal(err)
				}
			}
			r := newRunner(t, c, cutoff)
			beforeT, beforeE := snapshot(t, c, traces), snapshot(t, c, events)
			n, _, err := r.one(context.Background(), 0, 1, true)
			if err != nil {
				t.Fatal(err)
			}
			want := 0
			if tc.eligible {
				want = 1
			}
			if n != want {
				t.Fatalf("pairs=%d want=%d", n, want)
			}
			removed := map[int64]bool{}
			if tc.eligible {
				removed[1] = true
			}
			assertUnchanged(t, beforeT, snapshot(t, c, traces), removed)
			assertUnchanged(t, beforeE, snapshot(t, c, events), removed)
		})
	}
}

func TestForeignKeyAddedAfterPreflightStopsBeforeDeletion(t *testing.T) {
	c, cutoff := fixture(t)
	r := newRunner(t, c, cutoff)
	beforeT, beforeE := snapshot(t, c, traces), snapshot(t, c, events)
	_, err := c.Exec(context.Background(), `CREATE TABLE public.foreign_ref (trace_id bigint REFERENCES `+traces+`(id) ON DELETE CASCADE)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = r.one(context.Background(), 0, 1, true); err == nil {
		t.Fatal("new foreign key must block cleanup")
	}
	assertUnchanged(t, beforeT, snapshot(t, c, traces), nil)
	assertUnchanged(t, beforeE, snapshot(t, c, events), nil)
}
