package testenv

import (
	"net/http"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/instrumentation"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	integrationhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelig "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	repoig "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Env 提供集成网关测试环境。
type Env struct {
	T       testing.TB
	DB      *gorm.DB
	Service *manager.Service
	Bus     event_bus.EventBus
	Deps    *shared.Deps
}

// New 构造测试环境。
func New(t testing.TB) *Env {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true, // 在 SQLite 测试中禁用外键约束
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	coremodel.PowerXSchema = "main"
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	if err := db.AutoMigrate(
		&modelig.IntegrationRoute{},
		&modelig.IntegrationRouteVersion{},
		&modelig.IntegrationInvocationLog{},
		&modelig.IntegrationEventPublication{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	bus := event_bus.NewLocalEventBus()

	inst := instrumentation.NewInstrumentation(nil)

	routeRepo := repoig.NewIntegrationRouteRepository(db)
	versionRepo := repoig.NewIntegrationRouteVersionRepository(db)
	eventRepo := repoig.NewIntegrationEventPublicationRepository(db)

	svc := manager.NewService(manager.ServiceOptions{
		DB:              db,
		RouteRepo:       routeRepo,
		VersionRepo:     versionRepo,
		EventRepo:       eventRepo,
		EventBus:        bus,
		Instrumentation: inst,
		Auditor:         audit.Noop{},
		Config: manager.Config{
			RateLimitPrefix: "integration_gateway:rl",
			DefaultRateLimit: manager.RateLimitPolicy{
				Limit:         120,
				Burst:         120,
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
		Clock: time.Now,
	})

	deps := &shared.Deps{
		EventBus: bus,
		IntegrationGateway: &shared.IntegrationGatewayDeps{
			Manager:         svc,
			Instrumentation: inst,
		},
	}

	return &Env{T: t, DB: db, Service: svc, Bus: bus, Deps: deps}
}

// Close 释放资源。
func (e *Env) Close() {
	if e.Bus != nil {
		_ = e.Bus.Close()
	}
}

// Engine 返回注册了管理端路由的 gin 引擎。
func (e *Env) Engine() *gin.Engine {
	engine := gin.New()
	public := engine.Group("/api")
	protected := engine.Group("/api")
	protected.Use(func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	})

	integrationhttp.RegisterAPIRoutes(public, protected, e.Deps)
	return engine
}
