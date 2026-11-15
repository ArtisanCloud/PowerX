package taskqueue

import (
	"context"
	"testing"

	cap "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/stretchr/testify/require"
)

// Exercise actual typed queue responses rather than a synthetic JSON snapshot:
// idle -> submission -> lease -> settlement -> idle. Business task persistence
// and all non-idle invocation evidence must survive the idle audit policy.
func TestRuntimeQueueAuditsWorkWithoutPersistingIdleReads(t *testing.T) {
	driver, db, _ := fixture(t, "fixture_environment")
	require.NoError(t, db.AutoMigrate(&models.InvocationTrace{}, &models.CapabilityEventPublication{}))
	service := NewRuntimeService(driver)
	audit := cap.NewAuditService(cap.AuditServiceOptions{
		TraceRepo: repo.NewInvocationTraceRepository(db), EventRepo: repo.NewCapabilityEventPublicationRepository(db),
	})
	ctx := context.Background()
	scope := RuntimeScope{TenantUUID: "11111111-1111-4111-8111-111111111111", SourceService: "fixture_service"}
	in := RuntimeRequest{ContractVersion: RuntimeContract, Operation: "dequeue", TenantKey: scope.TenantUUID, SubscriberID: "fixture_subscriber", WorkerID: "fixture_worker", MaxItems: 1, WaitTimeoutMS: 1}
	apply := func(trace, capability string, input RuntimeRequest) map[string]interface{} {
		t.Helper()
		out, err := service.Apply(ctx, scope, input)
		require.NoError(t, err)
		audit.RecordInvocation(ctx, cap.InvocationAuditInput{
			TraceID: trace, TenantUUID: scope.TenantUUID, PluginID: "corex.platform", CapabilityID: capability,
			ProtocolUsed: "core_internal", Status: "completed", RequestPayload: map[string]interface{}{"body": input}, ResponsePayload: out,
		})
		return out
	}
	checkCount := func(want int64) {
		t.Helper()
		for _, model := range []interface{}{&models.InvocationTrace{}, &models.CapabilityEventPublication{}} {
			var count int64
			require.NoError(t, db.Model(model).Count(&count).Error)
			require.Equal(t, want, count)
		}
	}
	for i := 0; i < 3; i++ {
		require.Empty(t, apply("idle-before", ConsumeCapability, in)["messages"])
	}
	checkCount(0)
	msg := runtimeFixtureMessage()
	msg.TenantKey = scope.TenantUUID
	apply("enqueue", EnqueueCapability, RuntimeRequest{ContractVersion: RuntimeContract, Operation: "enqueue", TenantKey: scope.TenantUUID, Message: msg})
	checkCount(1)
	leased := apply("lease", ConsumeCapability, in)["messages"].([]RuntimeMessage)
	require.Len(t, leased, 1)
	checkCount(2)
	apply("ack", ConsumeCapability, RuntimeRequest{ContractVersion: RuntimeContract, Operation: "ack", TenantKey: scope.TenantUUID, SubscriberID: in.SubscriberID, MessageID: leased[0].ID, ReceiptUUID: leased[0].Lease.ReceiptUUID, Outcome: "completed", Metadata: map[string]string{"plugin_job_status": "succeeded"}})
	checkCount(3)
	require.Empty(t, apply("idle-after", ConsumeCapability, in)["messages"])
	checkCount(3)
	var businessCount int64
	require.NoError(t, db.Table("main.event_task_histories").Count(&businessCount).Error)
	require.EqualValues(t, 1, businessCount)
}
