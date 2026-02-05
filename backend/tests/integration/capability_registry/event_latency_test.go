package capabilityregistryintegration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/eventbus"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/stretchr/testify/require"
)

// TestIntegrationGatewayInvocationEventsDeliveredWithinOneMinute 验证 invocation 事件 95% 在 60s 内送达。
func TestIntegrationGatewayInvocationEventsDeliveredWithinOneMinute(t *testing.T) {
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
	traceRepo := repo.NewInvocationTraceRepository(env.DB)
	eventRepo := repo.NewCapabilityEventPublicationRepository(env.DB)

	catalogSvc := capservice.NewRegistryService(capservice.RegistryServiceOptions{
		RecordRepo:   recordRepo,
		TemplateRepo: templateRepo,
		JobRepo:      jobRepo,
	})
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
		CapabilityID: "cap.event.latency",
		TenantUUID:   "8091a2b3-8888-4141-9c9c-9999aaaabbbb",
		ContractRef:  "contracts/exposure/mcp-tools.json",
		Status:       string(domain.RegistrationStatusPublished),
		Adapters: []registry.AdapterEndpoint{
			{
				AdapterID:     "adapter-event-grpc",
				TransportType: "grpc",
				Endpoint:      "grpc://plugin.event.CapabilityService/Invoke",
				ServiceRef:    "plugin-event",
				Weight:        100,
				TimeoutMS:     1500,
			},
		},
		RoutingPolicy: registry.RoutingPolicy{
			Strategy:        string(domain.RoutingStrategyWeightedRoundRobin),
			CooldownSeconds: 30,
		},
	}

	env.simulateWorkerSync(t, ctx, payload)
	seedCapabilityRecord(t, ctx, recordRepo, payload, "plugin.event")

	const totalRequests = 20
	deliveries := make(chan time.Duration, totalRequests)
	startTimes := make(map[string]time.Time)
	var startMu sync.RWMutex

	unsub := env.Bus.Subscribe(eventbus.TopicIntegrationGatewayInvocationSucceeded, func(evt event_bus.Event) error {
		payloadMap, ok := evt.Payload.(map[string]interface{})
		if !ok {
			return nil
		}
		traceID, _ := payloadMap["trace_id"].(string)
		if traceID == "" {
			return nil
		}
		startMu.RLock()
		start := startTimes[traceID]
		startMu.RUnlock()
		if start.IsZero() {
			return nil
		}
		deliveries <- time.Since(start)
		return nil
	})
	t.Cleanup(unsub)

	for i := 0; i < totalRequests; i++ {
		traceID := fmt.Sprintf("event-latency-%02d", i)
		startMu.Lock()
		startTimes[traceID] = time.Now()
		startMu.Unlock()

		_, err := invocationSvc.Invoke(ctx, capservice.InvocationInput{
			CapabilityID:      payload.CapabilityID,
			TenantUUID:        payload.TenantUUID,
			PreferredProtocol: "grpc",
			TraceID:           traceID,
			IdempotencyKey:    fmt.Sprintf("event-%02d", i),
			Payload: map[string]interface{}{
				"prompt": fmt.Sprintf("event-latency-%d", i),
			},
		})
		require.NoError(t, err, "invocation %d should complete", i+1)
	}

	withinMinute := 0
	collected := 0
	timeout := time.After(5 * time.Second)
	for collected < totalRequests {
		select {
		case duration := <-deliveries:
			if duration <= time.Minute {
				withinMinute++
			}
			collected++
		case <-timeout:
			t.Fatalf("等待事件投递超时，仅收到 %d/%d", collected, totalRequests)
		}
	}

	ratio := float64(withinMinute) / float64(totalRequests)
	require.GreaterOrEqual(t, ratio, 0.95, "事件延迟合规比例需 ≥95%%，当前 %.2f", ratio)
}
