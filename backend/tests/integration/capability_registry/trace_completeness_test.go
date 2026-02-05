package capabilityregistryintegration

import (
	"context"
	"fmt"
	"testing"
	"time"

	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// TestInvocationTracesReachNinetyFivePercentWithinOneMinute 确认批量调用后 95% Trace 可在 60s 内写入。
func TestInvocationTracesReachNinetyFivePercentWithinOneMinute(t *testing.T) {
	env := newCapabilityRegistryEnv(t)
	t.Cleanup(env.Close)

	ctx := context.Background()
	require.NoError(t, env.DB.AutoMigrate(
		&models.CapabilityRecord{},
		&models.WorkflowTemplateRef{},
		&models.InvocationTrace{},
		&models.CapabilityEventPublication{},
	))

	recordRepo := repo.NewCapabilityRecordRepository(env.DB, nil)
	templateRepo := repo.NewWorkflowTemplateRepository(env.DB)
	jobRepo := repo.NewCapabilitySyncJobRepository(env.DB)

	catalogSvc := capservice.NewRegistryService(capservice.RegistryServiceOptions{
		RecordRepo:   recordRepo,
		TemplateRepo: templateRepo,
		JobRepo:      jobRepo,
	})

	traceRepo := repo.NewInvocationTraceRepository(env.DB)
	eventRepo := repo.NewCapabilityEventPublicationRepository(env.DB)
	auditSvc := capservice.NewAuditService(capservice.AuditServiceOptions{
		TraceRepo: traceRepo,
		EventRepo: eventRepo,
		EventBus:  env.Bus,
	})

	invocationSvc := capservice.NewInvocationService(capservice.InvocationServiceOptions{
		Catalog:   catalogSvc,
		Router:    env.RouterSvc,
		TraceRepo: traceRepo,
		EventRepo: eventRepo,
		EventBus:  env.Bus,
		Audit:     auditSvc,
	})

	payload := registry.RegistrationPayload{
		CapabilityID: "cap.trace.demo",
		TenantUUID:   "91a2b3c4-9999-4242-9d9d-aaaabbbbcccc",
		ContractRef:  "contracts/exposure/mcp-tools.json",
		Status:       string(domain.RegistrationStatusPublished),
		Adapters: []registry.AdapterEndpoint{
			{
				AdapterID:     "adapter-trace-grpc",
				TransportType: "grpc",
				Endpoint:      "grpc://plugin.trace.CapabilityService/Invoke",
				ServiceRef:    "plugin-trace",
				Weight:        100,
				TimeoutMS:     2000,
			},
		},
		RoutingPolicy: registry.RoutingPolicy{
			Strategy:        string(domain.RoutingStrategyWeightedRoundRobin),
			CooldownSeconds: 30,
		},
	}

	env.simulateWorkerSync(t, ctx, payload)
	seedCapabilityRecord(t, ctx, recordRepo, payload, "plugin.trace")

	const totalRequests = 20
	start := time.Now()
	for i := 0; i < totalRequests; i++ {
		_, err := invocationSvc.Invoke(ctx, capservice.InvocationInput{
			CapabilityID:      payload.CapabilityID,
			TenantUUID:        payload.TenantUUID,
			PreferredProtocol: "grpc",
			IdempotencyKey:    fmt.Sprintf("trace-%02d", i),
			Payload: map[string]interface{}{
				"prompt": fmt.Sprintf("trace-completeness-%d", i),
			},
		})
		require.NoError(t, err, "invocation %d should succeed", i+1)
	}

	traces, err := traceRepo.List(ctx, repo.InvocationTraceFilter{
		TenantUUID: payload.TenantUUID,
	})
	require.NoError(t, err)
	require.Len(t, traces, totalRequests, "所有调用都应记录 Trace")

	deadline := start.Add(time.Minute)
	withinMinute := 0
	for _, trace := range traces {
		require.False(t, trace.CreatedAt.IsZero(), "trace 需要创建时间以供审计")
		if !trace.CreatedAt.After(deadline) && !trace.CreatedAt.Before(start) {
			withinMinute++
		}
	}

	ratio := float64(withinMinute) / float64(totalRequests)
	require.GreaterOrEqual(t, ratio, 0.95, "至少 95%% 的 Trace 应在 60s 内写入（实际 %.2f）", ratio)
}

func seedCapabilityRecord(
	t *testing.T,
	ctx context.Context,
	repository *repo.CapabilityRecordRepository,
	payload registry.RegistrationPayload,
	pluginID string,
) {
	t.Helper()

	now := time.Now()
	record := &models.CapabilityRecord{
		CapabilityID:  payload.CapabilityID,
		PluginID:      pluginID,
		PluginVersion: "1.0.0",
		Title:         "Trace Completeness Demo",
		Description:   "used by trace completeness integration test",
		Categories:    datatypes.JSON([]byte(`["test"]`)),
		Intents:       datatypes.JSON([]byte(`["intent.trace"]`)),
		ToolScope:     datatypes.JSON([]byte(`["scope.trace"]`)),
		Protocols: datatypes.JSON([]byte(`[
			{"channel":"grpc","endpoint":"grpc://plugin.trace.CapabilityService/Invoke","rpc":"Invoke"}
		]`)),
		Policy:           datatypes.JSON([]byte(`{"prefer":"grpc","fallback":["grpc"]}`)),
		CapabilitiesHash: "trace-hash-v1",
		ProtocolHash:     "proto-hash-v1",
		Status:           "published",
		PublishedAt:      &now,
		CreatedBy:        "sync-worker",
		UpdatedBy:        "sync-worker",
	}

	_, err := repository.Upsert(ctx, record)
	require.NoError(t, err)
}
