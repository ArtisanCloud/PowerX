package taskqueue

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/stretchr/testify/require"
)

func runtimeFixtureMessage() *RuntimeMessage {
	return &RuntimeMessage{ID: "fixture_job", TenantKey: "fixture_tenant", SubscriberID: "fixture_subscriber", Topic: "fixture_topic", PayloadBase64: base64.StdEncoding.EncodeToString([]byte(`{"value":1}`)), IdempotencyKey: "fixture_idempotency", Metadata: map[string]string{"job_type": "fixture_job_type"}}
}
func TestRuntimeQueueContractImmutableSubmissionAndSourceIsolation(t *testing.T) {
	d, db, _ := fixture(t, "fixture_environment")
	s := NewRuntimeService(d)
	ctx := context.Background()
	scope := RuntimeScope{TenantUUID: "fixture_tenant", SourceService: "fixture_service"}
	msg := runtimeFixtureMessage()
	input := RuntimeRequest{ContractVersion: RuntimeContract, Operation: "enqueue", TenantKey: scope.TenantUUID, Message: msg}
	for n := 0; n < 2; n++ {
		out, err := s.Apply(ctx, scope, input)
		require.NoError(t, err)
		require.Equal(t, true, out["accepted"])
	}
	for _, change := range []func(*RuntimeMessage){func(m *RuntimeMessage) { m.ID = "fixture_other_id" }, func(m *RuntimeMessage) { m.PayloadBase64 = base64.StdEncoding.EncodeToString([]byte(`{"value":2}`)) }, func(m *RuntimeMessage) { m.IdempotencyKey = "fixture_other_key" }} {
		changed := *msg
		change(&changed)
		input.Message = &changed
		_, err := s.Apply(ctx, scope, input)
		require.Equal(t, "TASK_QUEUE_IDEMPOTENCY_CONFLICT", dto.CodeOf(err))
	}
	input.Message = msg
	_, err := s.Apply(ctx, RuntimeScope{TenantUUID: scope.TenantUUID, SourceService: "fixture_other_service"}, input)
	require.NoError(t, err)
	var count int64
	require.NoError(t, db.Table("main.event_task_histories").Count(&count).Error)
	require.EqualValues(t, 2, count)
}
func TestRuntimeQueueReceiptSettlementAndRenewal(t *testing.T) {
	d, _, _ := fixture(t, "fixture_environment")
	s := NewRuntimeService(d)
	ctx := context.Background()
	scope := RuntimeScope{TenantUUID: "fixture_tenant", SourceService: "fixture_service"}
	_, err := s.Apply(ctx, scope, RuntimeRequest{ContractVersion: RuntimeContract, Operation: "enqueue", TenantKey: scope.TenantUUID, Message: runtimeFixtureMessage()})
	require.NoError(t, err)
	out, err := s.Apply(ctx, scope, RuntimeRequest{ContractVersion: RuntimeContract, Operation: "dequeue", TenantKey: scope.TenantUUID, SubscriberID: "fixture_subscriber", WorkerID: "fixture_worker", MaxItems: 1, WaitTimeoutMS: 1})
	require.NoError(t, err)
	items := out["messages"].([]RuntimeMessage)
	require.Len(t, items, 1)
	require.NotNil(t, items[0].Lease)
	in := RuntimeRequest{ContractVersion: RuntimeContract, Operation: "renew", TenantKey: scope.TenantUUID, SubscriberID: items[0].SubscriberID, MessageID: items[0].ID, ReceiptUUID: items[0].Lease.ReceiptUUID}
	renewed, err := s.Apply(ctx, scope, in)
	require.NoError(t, err)
	require.Greater(t, renewed["lease"].(RuntimeLease).ExpiresAt, items[0].Lease.ExpiresAt)
	_, err = s.Apply(ctx, RuntimeScope{TenantUUID: scope.TenantUUID, SourceService: "fixture_other_service"}, in)
	require.Equal(t, "TASK_QUEUE_RECEIPT_STALE", dto.CodeOf(err))
	in.Operation = "ack"
	in.Outcome = "completed"
	in.Metadata = map[string]string{"plugin_job_status": "succeeded"}
	_, err = s.Apply(ctx, scope, in)
	require.NoError(t, err)
	out, err = s.Apply(ctx, scope, in)
	require.NoError(t, err)
	require.Equal(t, true, out["already_settled"])
	in.Operation = "renew"
	in.Outcome = ""
	in.Metadata = nil
	_, err = s.Apply(ctx, scope, in)
	require.Equal(t, "TASK_QUEUE_RECEIPT_STALE", dto.CodeOf(err))
}
func TestRuntimeQueueOldReceiptCannotAcknowledgeReclaimedDelivery(t *testing.T) {
	d, _, mini := fixture(t, "fixture_environment")
	s := NewRuntimeService(d)
	ctx := context.Background()
	scope := RuntimeScope{TenantUUID: "fixture_tenant", SourceService: "fixture_service"}
	_, err := s.Apply(ctx, scope, RuntimeRequest{ContractVersion: RuntimeContract, Operation: "enqueue", TenantKey: scope.TenantUUID, Message: runtimeFixtureMessage()})
	require.NoError(t, err)
	in := RuntimeRequest{ContractVersion: RuntimeContract, Operation: "dequeue", TenantKey: scope.TenantUUID, SubscriberID: "fixture_subscriber", WorkerID: "fixture_worker_a", MaxItems: 1, WaitTimeoutMS: 1}
	first, err := s.Apply(ctx, scope, in)
	require.NoError(t, err)
	a := first["messages"].([]RuntimeMessage)
	require.Len(t, a, 1)
	mini.FastForward(2 * time.Second)
	time.Sleep(1100 * time.Millisecond)
	in.WorkerID = "fixture_worker_b"
	second, err := s.Apply(ctx, scope, in)
	require.NoError(t, err)
	b := second["messages"].([]RuntimeMessage)
	require.Len(t, b, 1)
	require.NotEqual(t, a[0].Lease.ReceiptUUID, b[0].Lease.ReceiptUUID)
	require.Greater(t, b[0].Lease.Fence, a[0].Lease.Fence)
	ack := RuntimeRequest{ContractVersion: RuntimeContract, Operation: "ack", TenantKey: scope.TenantUUID, SubscriberID: in.SubscriberID, MessageID: a[0].ID, ReceiptUUID: a[0].Lease.ReceiptUUID, Outcome: "completed", Metadata: map[string]string{"plugin_job_status": "succeeded"}}
	_, err = s.Apply(ctx, scope, ack)
	require.Equal(t, "TASK_QUEUE_RECEIPT_STALE", dto.CodeOf(err))
	ack.ReceiptUUID = b[0].Lease.ReceiptUUID
	_, err = s.Apply(ctx, scope, ack)
	require.NoError(t, err)
}
