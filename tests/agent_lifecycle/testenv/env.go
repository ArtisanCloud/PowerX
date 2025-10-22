package testenv

import (
	"net/http"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	agentinstr "github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle/instrumentation"
	agentgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/agentlifecycle"
	adminhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agentlifecycle"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Env 为 Agent Lifecycle 测试提供独立环境。
type Env struct {
	T      testing.TB
	DB     *gorm.DB
	Deps   *shared.Deps
	Bus    event_bus.EventBus
	Server agentgrpc.Server
}

// New 构造测试环境。
func New(t testing.TB) *Env {
	t.Helper()

	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	coremodel.PowerXSchema = "main"
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	if err := db.AutoMigrate(
		&agentmodel.Agent{},
		&agentmodel.AgentSetting{},
		&agentmodel.AgentKBBinding{},
		&agentmodel.AgentPluginLink{},
		&agentmodel.AgentChatSession{},
		&agentmodel.AgentChatMessage{},
		&agentmodel.AgentProfileLifecycle{},
		&agentmodel.AgentLifecycleEventRecord{},
		&agentmodel.AgentHealthSnapshotRecord{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	bus := event_bus.NewLocalEventBus()
	inst := agentinstr.New(agentinstr.Options{})

	profileRepo := agentrepo.NewAgentProfileLifecycleRepository(db)
	eventRepo := agentrepo.NewAgentLifecycleEventRepository(db)
	healthRepo := agentrepo.NewAgentHealthSnapshotRepository(db)

	service := agent_lifecycle.NewService(agent_lifecycle.ServiceOptions{
		ProfileRepo:     profileRepo,
		LifecycleRepo:   eventRepo,
		HealthRepo:      healthRepo,
		EventBus:        bus,
		Instrumentation: inst,
		Config: agent_lifecycle.Config{
			DefaultCapacityInstances: 3,
			EventTopics: agent_lifecycle.EventTopics{
				LifecyclePrefix: "agent.lifecycle",
				HealthPrefix:    "agent.health",
			},
		},
		Clock: time.Now,
	})

	agentDeps := &shared.AgentLifecycleDeps{
		ProfileRepo:     profileRepo,
		LifecycleRepo:   eventRepo,
		HealthRepo:      healthRepo,
		Instrumentation: inst,
		Notifications:   nil,
		RedisClient:     nil,
		EventBus:        bus,
		Config: shared.AgentLifecycleRuntimeConfig{
			CapacityKeyPrefix:        "agent_lifecycle:capacity",
			HealthKeyPrefix:          "agent_lifecycle:health",
			DefaultCapacityInstances: 3,
			EventTopics: shared.AgentLifecycleEventTopicsOptions{
				LifecyclePrefix: "agent.lifecycle",
				HealthPrefix:    "agent.health",
			},
		},
		Service: service,
	}

	deps := &shared.Deps{
		EventBus:       bus,
		AgentLifecycle: agentDeps,
	}

	return &Env{
		T:      t,
		DB:     db,
		Deps:   deps,
		Bus:    bus,
		Server: agentgrpc.NewServer(service),
	}
}

// Close 释放资源。
func (e *Env) Close() {
	if e.Bus != nil {
		_ = e.Bus.Close()
	}
}

// Engine 返回注册了生命周期路由的 gin 引擎。
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
	adminhttp.Register(public, protected, e.Deps)
	return engine
}

// GRPCServer 返回实现 AgentLifecycleService 的 gRPC Server。
func (e *Env) GRPCServer() grpc.ServiceRegistrar {
	server := grpc.NewServer()
	agentgrpc.Register(server, e.Server)
	return server
}

// SeedAgent 快速创建一个代理档案。
func (e *Env) SeedAgent(tenantID, alias string) uuid.UUID {
	profile := &agentmodel.AgentProfileLifecycle{
		TenantID:    tenantID,
		Alias:       alias,
		DisplayName: alias,
		Status:      "pending",
	}
	if err := e.DB.Create(profile).Error; err != nil {
		e.T.Fatalf("seed agent: %v", err)
	}
	return profile.UUID
}
