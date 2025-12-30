package integrationgatewaycontract

import (
	"context"
	"errors"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	registrymodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	registryrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestCapabilitySyncWorkerAssetValidation emulates the capability sync worker writing payloads
// into the registry service, asserting that invalid assets get rejected (simulating sync_failed)
// while valid assets persist to the repository.
func TestCapabilitySyncWorkerAssetValidation(t *testing.T) {
	env := newCapabilityRegistryWorkerEnv(t)
	t.Cleanup(env.Close)

	ctx := context.Background()

	t.Run("missing contract reference", func(t *testing.T) {
		payload := registry.RegistrationPayload{
			CapabilityID: "cap.invalid.contract",
			TenantUUID:   "tenant-worker-001",
			Status:       string(domain.RegistrationStatusPublished),
			Adapters: []registry.AdapterEndpoint{
				{
					AdapterID:     "adapter-grpc",
					TransportType: "grpc",
					Endpoint:      "grpc://plugin.invalid/Invoke",
					Weight:        100,
					TimeoutMS:     2000,
				},
			},
			RoutingPolicy: registry.RoutingPolicy{
				Strategy:        string(domain.RoutingStrategyWeightedRoundRobin),
				CooldownSeconds: 60,
			},
		}
		_, err := env.Service.CreateRegistration(ctx, registry.CreateRegistrationInput{
			Registration: payload,
			Actor:        "sync-worker",
		})
		require.Error(t, err)
		require.True(t, errors.Is(err, registry.ErrInvalidPayload))

		_, repoErr := env.Repository.GetLatest(ctx, nil, payload.CapabilityID, payload.TenantUUID)
		require.Error(t, repoErr, "invalid payload must not be persisted")
	})

	t.Run("missing adapters", func(t *testing.T) {
		payload := registry.RegistrationPayload{
			CapabilityID: "cap.invalid.adapters",
			TenantUUID:   "tenant-worker-002",
			ContractRef:  "contracts/exposure/mcp-tools.json",
			Status:       string(domain.RegistrationStatusPublished),
			RoutingPolicy: registry.RoutingPolicy{
				Strategy:        string(domain.RoutingStrategyPriority),
				CooldownSeconds: 60,
			},
		}
		_, err := env.Service.CreateRegistration(ctx, registry.CreateRegistrationInput{
			Registration: payload,
			Actor:        "sync-worker",
		})
		require.Error(t, err)
		require.True(t, errors.Is(err, registry.ErrInvalidPayload))
	})

	t.Run("valid payload persists", func(t *testing.T) {
		payload := registry.RegistrationPayload{
			CapabilityID: "cap.valid.sync",
			TenantUUID:   "tenant-worker-003",
			ContractRef:  "contracts/exposure/workflow/template.json",
			Status:       string(domain.RegistrationStatusPublished),
			Adapters: []registry.AdapterEndpoint{
				{
					AdapterID:     "adapter-primary",
					TransportType: "grpc",
					Endpoint:      "grpc://plugin.valid/Invoke",
					ServiceRef:    "plugin-valid",
					Weight:        100,
					TimeoutMS:     3000,
				},
			},
			RoutingPolicy: registry.RoutingPolicy{
				Strategy:        string(domain.RoutingStrategyWeightedRoundRobin),
				CooldownSeconds: 60,
				RateLimit: &registry.RateLimit{
					Limit:         120,
					WindowSeconds: 60,
				},
			},
		}

		reg, err := env.Service.CreateRegistration(ctx, registry.CreateRegistrationInput{
			Registration: payload,
			Actor:        "sync-worker",
		})
		require.NoError(t, err)
		require.Equal(t, uint64(1), reg.Version)

		stored, err := env.Repository.GetLatest(ctx, nil, payload.CapabilityID, payload.TenantUUID)
		require.NoError(t, err)
		require.Equal(t, reg.Version, stored.Version)
		require.Equal(t, len(payload.Adapters), len(stored.Adapters))
	})
}

type capabilityRegistryWorkerEnv struct {
	DB         *gorm.DB
	Service    *registry.Service
	Repository registry.Repository
	EventBus   event_bus.EventBus
}

func newCapabilityRegistryWorkerEnv(t *testing.T) *capabilityRegistryWorkerEnv {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	prevSchema := model.PowerXSchema
	model.PowerXSchema = "main"
	t.Cleanup(func() {
		model.PowerXSchema = prevSchema
	})
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	require.NoError(t, db.AutoMigrate(
		&registrymodel.CapabilityRegistration{},
		&registrymodel.AdapterEndpoint{},
		&registrymodel.RoutingPolicy{},
		&registrymodel.FallbackPlan{},
	))

	bus := event_bus.NewLocalEventBus()
	svc := registry.NewService(registry.ServiceOptions{
		DB:              db,
		EventBus:        bus,
		Instrumentation: domain.NewInstrumentation(nil),
	})

	repo := registryrepo.NewCapabilityRegistryRepository(db)

	return &capabilityRegistryWorkerEnv{
		DB:         db,
		Service:    svc,
		Repository: repo,
		EventBus:   bus,
	}
}

func (e *capabilityRegistryWorkerEnv) Close() {
	if e.EventBus != nil {
		_ = e.EventBus.Close()
	}
	if e.DB != nil {
		if sqlDB, err := e.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}
