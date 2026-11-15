package capability_registry

import (
	"context"
	"fmt"
	"testing"
	"time"

	capmetrics "github.com/ArtisanCloud/PowerX/internal/observability/metrics"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/stretchr/testify/require"
)

type idleAuditMetric struct {
	name  string
	value float64
	attrs map[string]string
}

type idleAuditEmitter struct{ samples []idleAuditMetric }

func (m *idleAuditEmitter) Counter(_ context.Context, name string, value float64, attrs map[string]string) {
	m.samples = append(m.samples, idleAuditMetric{name, value, attrs})
}
func (m *idleAuditEmitter) Histogram(ctx context.Context, name string, value float64, attrs map[string]string) {
	m.Counter(ctx, name, value, attrs)
}

type idleAuditBus struct {
	event_bus.EventBus
	published int
}

func (b *idleAuditBus) Publish(_ string, _ interface{}, _ context.Context) { b.published++ }

func idleInvocationForAudit() InvocationAuditInput {
	const tenant = "11111111-1111-4111-8111-111111111111"
	return InvocationAuditInput{
		TraceID: "idle-trace", TenantUUID: tenant, PluginID: "corex.platform",
		CapabilityID:      "com.corex.runtime.taskqueue.service_consume",
		PreferredProtocol: "core_internal", ProtocolUsed: "core_internal", Status: "completed",
		RequestPayload: map[string]interface{}{"body": map[string]interface{}{
			"contract_version": "taskqueue.v2", "operation": "dequeue", "tenant_key": tenant,
			"subscriber_id": "preview", "worker_id": "worker-1", "max_items": 1, "wait_timeout_ms": 50,
		}},
		ResponsePayload: map[string]interface{}{"contract_version": "taskqueue.v2", "messages": []interface{}{}},
		Latency:         50 * time.Millisecond,
	}
}

func TestAuditIdleTaskQueueDoesNotWriteRowsOrPublishPerCall(t *testing.T) {
	db := newAuditMemoryDB(t)
	emitter := &idleAuditEmitter{}
	bus := &idleAuditBus{}
	audit := NewAuditService(AuditServiceOptions{
		TraceRepo: repo.NewInvocationTraceRepository(db), EventRepo: repo.NewCapabilityEventPublicationRepository(db),
		EventBus: bus, Metrics: capmetrics.NewCapabilityRegistryMetrics(emitter),
	})
	for i := 0; i < 100; i++ {
		input := idleInvocationForAudit()
		input.TraceID = fmt.Sprintf("idle-%d", i)
		audit.RecordInvocation(context.Background(), input)
		require.Equal(t, fmt.Sprintf("idle-%d", i), input.TraceID)
		require.Equal(t, "dequeue", input.RequestPayload["body"].(map[string]interface{})["operation"])
		require.Empty(t, input.ResponsePayload["messages"])
	}
	for _, model := range []interface{}{&models.InvocationTrace{}, &models.CapabilityEventPublication{}} {
		var count int64
		require.NoError(t, db.Model(model).Count(&count).Error)
		require.Zero(t, count, "idle observations must not produce durable rows")
	}
	require.Zero(t, bus.published)
	require.Len(t, emitter.samples, 200, "retain count and latency for all empty reads")
	for _, sample := range emitter.samples {
		require.NotContains(t, sample.attrs, capmetrics.LabelTraceID)
		require.Equal(t, "success", sample.attrs[capmetrics.LabelResult])
		require.Equal(t, "com.corex.runtime.taskqueue.service_consume", sample.attrs[capmetrics.LabelCapabilityID])
	}
}

func TestAuditIdleTaskQueueKeepsDurableAuditForOtherOutcomes(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*InvocationAuditInput)
	}{
		{"message leased", func(i *InvocationAuditInput) {
			i.ResponsePayload["messages"] = []interface{}{map[string]interface{}{"id": "job-1"}}
		}},
		{"failed empty read", func(i *InvocationAuditInput) { i.Status = "failed"; i.ErrorSummary = "queue unavailable" }},
		{"denied", func(i *InvocationAuditInput) { i.Status = "denied" }},
		{"unspecified status", func(i *InvocationAuditInput) { i.Status = "" }},
		{"error summary", func(i *InvocationAuditInput) { i.ErrorSummary = "unexpected partial result" }},
		{"fallback", func(i *InvocationAuditInput) { i.FallbackUsed = true }},
		{"idempotency key", func(i *InvocationAuditInput) { i.IdempotencyKey = "caller-key" }},
		{"plugin capability", func(i *InvocationAuditInput) { i.CapabilityID = "vendor.taskqueue.consume" }},
		{"other owner", func(i *InvocationAuditInput) { i.PluginID = "vendor.plugin" }},
		{"rest", func(i *InvocationAuditInput) { i.ProtocolUsed = "rest" }},
		{"missing tenant", func(i *InvocationAuditInput) { i.TenantUUID = "" }},
		{"invalid tenant", func(i *InvocationAuditInput) { i.TenantUUID = "tenant-1" }},
		{"nil uuid", func(i *InvocationAuditInput) { i.TenantUUID = "00000000-0000-0000-0000-000000000000" }},
		{"other tenant", func(i *InvocationAuditInput) {
			i.RequestPayload["body"].(map[string]interface{})["tenant_key"] = "22222222-2222-4222-8222-222222222222"
		}},
		{"ack", func(i *InvocationAuditInput) { i.RequestPayload["body"].(map[string]interface{})["operation"] = "ack" }},
		{"enqueue", func(i *InvocationAuditInput) {
			i.CapabilityID = "com.corex.runtime.taskqueue.service_enqueue"
			i.RequestPayload["body"].(map[string]interface{})["operation"] = "enqueue"
		}},
		{"old request contract", func(i *InvocationAuditInput) {
			i.RequestPayload["body"].(map[string]interface{})["contract_version"] = "taskqueue.v1"
		}},
		{"old response contract", func(i *InvocationAuditInput) { i.ResponsePayload["contract_version"] = "taskqueue.v1" }},
		{"missing body", func(i *InvocationAuditInput) { i.RequestPayload = nil }},
		{"caller skip flag", func(i *InvocationAuditInput) { i.RequestPayload["skip_audit"] = true }},
		{"extra response", func(i *InvocationAuditInput) { i.ResponsePayload["error"] = "partial" }},
		{"missing messages", func(i *InvocationAuditInput) { delete(i.ResponsePayload, "messages") }},
		{"null messages", func(i *InvocationAuditInput) { i.ResponsePayload["messages"] = nil }},
		{"nil slice", func(i *InvocationAuditInput) { i.ResponsePayload["messages"] = []interface{}(nil) }},
		{"byte slice", func(i *InvocationAuditInput) { i.ResponsePayload["messages"] = []byte{} }},
		{"empty object", func(i *InvocationAuditInput) { i.ResponsePayload["messages"] = map[string]interface{}{} }},
		{"snapshot is not execution proof", func(i *InvocationAuditInput) {
			i.ResponsePayload["messages"] = map[string]interface{}{"_omitted": "content", "size": 0}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newAuditMemoryDB(t)
			bus := &idleAuditBus{}
			emitter := &idleAuditEmitter{}
			audit := NewAuditService(AuditServiceOptions{TraceRepo: repo.NewInvocationTraceRepository(db), EventRepo: repo.NewCapabilityEventPublicationRepository(db), EventBus: bus, Metrics: capmetrics.NewCapabilityRegistryMetrics(emitter)})
			input := idleInvocationForAudit()
			tc.mutate(&input)
			audit.RecordInvocation(context.Background(), input)
			for _, model := range []interface{}{&models.InvocationTrace{}, &models.CapabilityEventPublication{}} {
				var count int64
				require.NoError(t, db.Model(model).Count(&count).Error)
				require.EqualValues(t, 1, count, "preserve audit and notification")
			}
			require.Positive(t, bus.published)
			require.Equal(t, input.TraceID, emitter.samples[0].attrs[capmetrics.LabelTraceID])
		})
	}
}
