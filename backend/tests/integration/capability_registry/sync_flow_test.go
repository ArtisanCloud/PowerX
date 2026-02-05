package capabilityregistryintegration

import (
	"context"
	"testing"
	"time"

	domain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCapabilityRegistrySyncFlow(t *testing.T) {
	env := newCapabilityRegistryEnv(t)
	t.Cleanup(env.Close)

	ctx := context.Background()

	// 订阅 registry updated 事件，模拟 Worker 完成同步后的广播。
	eventCh := make(chan event_bus.Event, 1)
	unsub := env.Bus.Subscribe("capability.registry.updated", func(evt event_bus.Event) error {
		select {
		case eventCh <- evt:
		default:
		}
		return nil
	})
	t.Cleanup(unsub)

	payload := registry.RegistrationPayload{
		CapabilityID: "cap.sync.demo",
		TenantUUID:   "2a3b4c5d-2222-4b4b-8c8c-333344445555",
		ContractRef:  "contracts/exposure/mcp-tools.json",
		Status:       string(domain.RegistrationStatusPublished),
		EnvironmentPolicies: map[string]registry.EnvironmentPolicy{
			"default": {
				IsEnabled: true,
				Overrides: map[string]string{"env": "prod"},
			},
		},
		ToolGrantIDs: []string{"grant-alpha"},
		Adapters: []registry.AdapterEndpoint{
			{
				AdapterID:     "adapter-grpc-primary",
				TransportType: "grpc",
				Endpoint:      "grpc://plugin.demo.CapabilityService/Invoke",
				ServiceRef:    "plugin-demo",
				Weight:        100,
				TimeoutMS:     3000,
				Labels: map[string]string{
					"protocol": "grpc",
				},
				IsActive: true,
			},
		},
		RoutingPolicy: registry.RoutingPolicy{
			Strategy:        string(domain.RoutingStrategyWeightedRoundRobin),
			CooldownSeconds: 60,
			RateLimit: &registry.RateLimit{
				Limit:         100,
				WindowSeconds: 60,
			},
		},
	}

	reg := env.simulateWorkerSync(t, ctx, payload)
	require.Equal(t, uint64(1), reg.Version)
	require.Equal(t, payload.CapabilityID, reg.CapabilityID)

	// Admin/Discovery API 读取到与 Worker 同步一致的快照。
	stored, err := env.RegistrySvc.GetRegistration(ctx, reg.CapabilityID, reg.TenantUUID, registry.GetRegistrationOptions{})
	require.NoError(t, err)
	require.Equal(t, reg.Version, stored.Version)
	require.Len(t, stored.Adapters, 1)
	require.Equal(t, "grpc", stored.Adapters[0].TransportType)

	// Tenant 调用时，Router 直接消费同一份 Registry 数据。
	result, err := env.RouterSvc.Invoke(ctx, router.InvokeRequest{
		CapabilityID: reg.CapabilityID,
		TenantUUID:   reg.TenantUUID,
	})
	require.NoError(t, err)
	require.Equal(t, "adapter-grpc-primary", result.AdapterID)
	require.Equal(t, "grpc", result.Transport)

	// Worker 触发的事件被监听到，验证 quickstart 中“广播通知 → 缓存刷新”的链路钩子已生效。
	select {
	case evt := <-eventCh:
		require.Equal(t, "capability.registry.updated", evt.Name)
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout waiting for capability.registry.updated event")
	}
}

type capabilityRegistryEnv struct {
	DB          *gorm.DB
	Bus         event_bus.EventBus
	RegistrySvc *registry.Service
	RouterSvc   *router.Service
}

func newCapabilityRegistryEnv(t *testing.T) *capabilityRegistryEnv {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = prevSchema
	})
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	require.NoError(t, db.AutoMigrate(
		&models.CapabilityRegistration{},
		&models.AdapterEndpoint{},
		&models.RoutingPolicy{},
		&models.FallbackPlan{},
	))

	bus := event_bus.NewLocalEventBus()
	inst := domain.NewInstrumentation(nil)

	registrySvc := registry.NewService(registry.ServiceOptions{
		DB:              db,
		EventBus:        bus,
		Instrumentation: inst,
	})
	routerSvc := router.NewService(router.ServiceOptions{
		DB:              db,
		EventBus:        bus,
		Instrumentation: inst,
	})

	return &capabilityRegistryEnv{
		DB:          db,
		Bus:         bus,
		RegistrySvc: registrySvc,
		RouterSvc:   routerSvc,
	}
}

func (e *capabilityRegistryEnv) Close() {
	if e.Bus != nil {
		_ = e.Bus.Close()
	}
	if e.DB != nil {
		if sqlDB, err := e.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}

func (e *capabilityRegistryEnv) simulateWorkerSync(t *testing.T, ctx context.Context, payload registry.RegistrationPayload) registry.Registration {
	t.Helper()
	reg, err := e.RegistrySvc.CreateRegistration(ctx, registry.CreateRegistrationInput{
		Registration: payload,
		Actor:        "sync-worker",
	})
	require.NoError(t, err)
	return reg
}
