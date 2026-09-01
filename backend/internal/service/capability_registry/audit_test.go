package capability_registry

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
)

func TestAuditServiceRecordInvocation(t *testing.T) {
	ctx := context.Background()
	db := newAuditMemoryDB(t)

	traceRepo := repo.NewInvocationTraceRepository(db)
	eventRepo := repo.NewCapabilityEventPublicationRepository(db)
	bus := event_bus.NewLocalEventBus()
	t.Cleanup(func() { _ = bus.Close() })

	audit := NewAuditService(AuditServiceOptions{
		TraceRepo: traceRepo,
		EventRepo: eventRepo,
		EventBus:  bus,
		Clock: func() time.Time {
			return time.Unix(1700000000, 0).UTC()
		},
	})

	events := make(chan event_bus.Event, 1)
	unsub := bus.Subscribe(eventbus.TopicIntegrationGatewayInvocationSucceeded, func(evt event_bus.Event) error {
		events <- evt
		return nil
	})
	t.Cleanup(unsub)

	audit.RecordInvocation(ctx, InvocationAuditInput{
		TraceID:           "trace-123",
		TenantUUID:        "tenant-001",
		PluginID:          "demo.plugin",
		CapabilityID:      "demo.capability",
		PreferredProtocol: "mcp",
		ProtocolUsed:      "mcp",
		Status:            "completed",
		RequestPayload:    map[string]interface{}{"prompt": "hello"},
		ResponsePayload:   map[string]interface{}{"result": "ok"},
		Latency:           50 * time.Millisecond,
	})

	select {
	case evt := <-events:
		payload, ok := evt.Payload.(map[string]any)
		require.True(t, ok)
		require.Equal(t, "demo.capability", payload["capability_id"])
		require.Equal(t, "trace-123", payload["trace_id"])
	case <-time.After(time.Second):
		t.Fatalf("expected invocation event")
	}

	trace, err := traceRepo.GetByTraceID(ctx, "trace-123")
	require.NoError(t, err)
	require.Equal(t, "demo.capability", trace.CapabilityID)
	require.Equal(t, "demo.plugin", trace.PluginID)
	require.Equal(t, 50, trace.LatencyMS)

	records, err := eventRepo.List(ctx, repo.CapabilityEventPublicationFilter{TenantUUID: "tenant-001"})
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "trace-123", records[0].TraceID)
}

func TestAuditServiceAllowsMultipleInvocationsInOneTrace(t *testing.T) {
	ctx := context.Background()
	db := newAuditMemoryDB(t)

	traceRepo := repo.NewInvocationTraceRepository(db)
	eventRepo := repo.NewCapabilityEventPublicationRepository(db)
	audit := NewAuditService(AuditServiceOptions{
		TraceRepo: traceRepo,
		EventRepo: eventRepo,
	})

	for _, capabilityID := range []string{"powerx.release.report_synthesis.execute", "com.corex.agent.stream"} {
		audit.RecordInvocation(ctx, InvocationAuditInput{
			TraceID:      "shared-trace-123",
			TenantUUID:   "tenant-001",
			PluginID:     "corex.platform",
			CapabilityID: capabilityID,
			ProtocolUsed: "core_internal",
			Status:       "completed",
		})
	}

	traces, err := traceRepo.List(ctx, repo.InvocationTraceFilter{
		TenantUUID: "tenant-001",
		OrderBy:    "created_at ASC, id ASC",
	})
	require.NoError(t, err)
	require.Len(t, traces, 2)
	require.Equal(t, "shared-trace-123", traces[0].TraceID)
	require.Equal(t, "shared-trace-123", traces[1].TraceID)
	require.NotEqual(t, traces[0].UUID, traces[1].UUID)
	require.True(t, traces[0].EventPublished)
	require.True(t, traces[1].EventPublished)
	require.NotNil(t, traces[0].EventPublicationID)
	require.NotNil(t, traces[1].EventPublicationID)
	require.NotEqual(t, *traces[0].EventPublicationID, *traces[1].EventPublicationID)
}

func newAuditMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = prevSchema
	})
	require.NoError(t, db.AutoMigrate(
		&models.InvocationTrace{},
		&models.CapabilityEventPublication{},
	))
	return db
}
