package capability_registry

import (
	"context"
	"testing"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/stretchr/testify/require"
)

// 降低调用频率不能通过静默丢弃必要审计实现；无发布确认也不能假装事件已完成。
func TestAuditBaselineReadsPreserveEachTraceAndUnknownPublicationState(t *testing.T) {
	db := newAuditMemoryDB(t)
	clock := time.Now().UTC().Truncate(time.Microsecond)
	audit := NewAuditService(AuditServiceOptions{
		TraceRepo: repo.NewInvocationTraceRepository(db), EventRepo: repo.NewCapabilityEventPublicationRepository(db),
		Clock: func() time.Time { return clock },
	})
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		audit.RecordInvocation(ctx, InvocationAuditInput{
			TraceID: "fixture-shared-request", TenantUUID: "fixture-tenant", PluginID: "corex.platform",
			CapabilityID: "com.corex.iam.authorization_baseline.service_read", ProtocolUsed: "core_internal", Status: "completed",
			RequestPayload: map[string]any{"operation": "read_member_baselines"}, ResponsePayload: map[string]any{"fixture_decision_revision": i},
		})
	}
	var traces []models.InvocationTrace
	require.NoError(t, db.Order("id").Find(&traces).Error)
	require.Len(t, traces, 4)
	for _, trace := range traces {
		require.Equal(t, models.InvocationAuditClass, trace.RecordClass)
		require.NotNil(t, trace.CompletedAt)
		require.NotNil(t, trace.EventPublicationID)
		require.False(t, trace.EventPublished)
	}
	var events []models.CapabilityEventPublication
	require.NoError(t, db.Find(&events).Error)
	require.Len(t, events, 4)
	for _, event := range events {
		require.Equal(t, models.InvocationNotificationClass, event.RecordClass)
		require.Equal(t, "unconfirmed", event.Status)
		require.Equal(t, 1, event.Attempts)
		require.Nil(t, event.TerminalAt)
		require.Nil(t, event.PublishTime)
		require.Empty(t, event.DeliveryReceipt)
	}
}
