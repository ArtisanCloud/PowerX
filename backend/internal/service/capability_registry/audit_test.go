package capability_registry

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/internal/eventbus"
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
