package workflowunit

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/instrumentation"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelig "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	repoig "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const validatorTenantUUID = "b784b3c2-4f5a-4c37-9d91-df9478a78d7a"

func newManagerService(t *testing.T) (*manager.Service, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true, // 在 SQLite 测试中禁用外键约束
	})
	require.NoError(t, err)
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)
	coremodel.PowerXSchema = "main" // 使用 "main" 以便 TableName() 返回不带前缀的表名
	require.NoError(t, db.AutoMigrate(
		&modelig.IntegrationRoute{},
		&modelig.IntegrationRouteVersion{},
		&modelig.IntegrationInvocationLog{},
		&modelig.IntegrationEventPublication{},
	))

	bus := event_bus.NewLocalEventBus()
	svc := manager.NewService(manager.ServiceOptions{
		DB:              db,
		RouteRepo:       repoig.NewIntegrationRouteRepository(db),
		VersionRepo:     repoig.NewIntegrationRouteVersionRepository(db),
		EventRepo:       repoig.NewIntegrationEventPublicationRepository(db),
		EventBus:        bus,
		Instrumentation: instrumentation.NewInstrumentation(nil),
		Auditor:         audit.Noop{},
		Config: manager.Config{
			RateLimitPrefix: "integration_gateway:rl",
			DefaultRateLimit: manager.RateLimitPolicy{
				Limit:         100,
				Burst:         100,
				WindowSeconds: 60,
				Scope:         "per_route_per_tenant",
			},
			EventTopics: manager.EventTopics{
				Created:             "integration.gateway.route.created",
				Updated:             "integration.gateway.route.updated",
				InvocationSucceeded: "integration.gateway.invocation.succeeded",
				InvocationFailed:    "integration.gateway.invocation.failed",
			},
		},
		Clock: func() time.Time { return time.Unix(1, 0).UTC() },
	})
	return svc, db
}

func TestCreateRouteSlugValidation(t *testing.T) {
	svc, _ := newManagerService(t)
	_, err := svc.CreateRoute(context.Background(), manager.CreateRouteInput{
		TenantUUID:   validatorTenantUUID,
		Actor:        "tester",
		RouteSlug:    "INVALID", // 大写不允许
		CapabilityID: "cap.demo",
		ToolGrantIDs: []string{"grant-demo"},
		Channels:     []string{"http"},
	})
	require.Error(t, err)
}

func TestCreateRouteDefaultRateLimitApplied(t *testing.T) {
	svc, db := newManagerService(t)
	ctx := context.Background()

	route, err := svc.CreateRoute(ctx, manager.CreateRouteInput{
		TenantUUID:   validatorTenantUUID,
		Actor:        "tester",
		RouteSlug:    "demo",
		CapabilityID: "cap.demo",
		ToolGrantIDs: []string{"grant-demo"},
		Channels:     []string{"http"},
		RateLimit:    nil,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(100), route.RateLimit.Limit)
	require.Equal(t, uint64(100), route.RateLimit.Burst)
	require.Equal(t, 60, route.RateLimit.WindowSeconds)

	var stored modelig.IntegrationRoute
	require.NoError(t, db.Where("uuid = ?", route.RouteID).First(&stored).Error)
	require.Equal(t, "per_route_per_tenant", route.RateLimit.Scope)
}

func TestChangeLifecycleCreatesVersionAndEvent(t *testing.T) {
	svc, db := newManagerService(t)
	ctx := context.Background()

	route, err := svc.CreateRoute(ctx, manager.CreateRouteInput{
		TenantUUID:   validatorTenantUUID,
		Actor:        "tester",
		RouteSlug:    "controller",
		CapabilityID: "cap.ctrl",
		ToolGrantIDs: []string{"grant-ctrl"},
		Channels:     []string{"http"},
	})
	require.NoError(t, err)

	updated, err := svc.ChangeLifecycle(ctx, manager.ChangeLifecycleInput{
		RouteID:    route.RouteID,
		TenantUUID: validatorTenantUUID,
		Actor:    "tester",
		Action:   "suspend",
	})
	require.NoError(t, err)
	require.Equal(t, manager.LifecycleSuspended, updated.LifecycleState)

	var versions int64
	require.NoError(t, db.Model(&modelig.IntegrationRouteVersion{}).Where("route_uuid = ?", route.RouteID).Count(&versions).Error)
	require.GreaterOrEqual(t, versions, int64(2)) // 创建 + suspend

	var events int64
	require.NoError(t, db.Model(&modelig.IntegrationEventPublication{}).Where("route_uuid = ?", route.RouteID).Count(&events).Error)
	require.GreaterOrEqual(t, events, int64(2))
}
