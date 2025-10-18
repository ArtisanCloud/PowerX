package shared

// internal/bootstrap/deps.go

import (
	"context"
	"strings"
	"time"

	discoverycache "github.com/ArtisanCloud/PowerX/internal/infra/cache/discovery"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	discoveryService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/discovery"
	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	capabilityHealth "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/health"
	capabilityRegistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	capabilityRouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	capabilitySandbox "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/sandbox"
	directoryService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	tenantsvc "github.com/ArtisanCloud/PowerX/internal/service/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	auditrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/audit"
	capabilityRegistryRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Deps struct {
	DB           *gorm.DB
	ctx          *context.Context
	AuthUser     *authsvc.AuthService
	AuthCustomer *authsvc.AuthService
	MeService    *authsvc.MeService

	//Bus    eventbus.Publisher // 来自 pkg/corex/event_bus

	AuditSvc auditsvc.Service // 底层批量写库 + sink
	Auditor  auditsvc.Auditor // 门面，兼容 LogAPI/LogRBAC 等调用

	TenantSvc *tenantsvc.TenantService
	MediaMgr  *mediamgr.MediaManager
	MediaSvc  *mediasvc.MediaService

	EventBus              event_bus.EventBus
	CapabilityRegistrySvc *capabilityRegistry.Service
	RouterSvc             *capabilityRouter.Service
	RouterSandboxSvc      *capabilitySandbox.Service
	DiscoverySvc          *discoveryService.Service

	EventFabric *EventFabricDeps
}

func NewDeps(db *gorm.DB, opts *DepsOptions) *Deps {
	ctx := context.Background()
	authUser := authsvc.NewAuthService(db, opts.AuthUser)
	authCustomer := authsvc.NewAuthService(db, opts.AuthCustomer)

	// --- Audit 初始化 ---
	dbRepo := auditrepo.NewAuditEventRepository(db) // 你已有的 GORM repo
	sinks := []auditsvc.Sink{&auditsvc.LoggerSink{L: pxlog.GetGlobalLogger()}}
	svc := auditsvc.NewService(dbRepo, sinks, opts.Audit)
	// 注册 GORM 回调
	auditsvc.RegisterAuditCallbacks(db, svc)

	aud := auditsvc.NewAuditor(svc)

	_ = svc.Emit(ctx, &dbm.AuditEvent{
		OccurredAt:   time.Now(),
		Source:       "selftest",
		Operation:    "BOOT",
		ResourceType: "system",
		ResourceID:   "bootstrap",
		Outcome:      "SUCCESS",
		Severity:     "INFO",
		Meta:         []byte(`{"msg":"audit self test"}`),
	})

	// --- AuthService ---
	meSvc := authsvc.NewMeService(db)

	// --- TenantService ---
	tenantSvc := tenantsvc.NewTenantService(db, authUser)

	// --- Media Manager & Service ---
	mediaManager, mediaSvc := mediasvc.BuildMediaStack(ctx, db, svc, opts.Storage)

	permSvc := iamsvc.NewPermissionService(db)
	if err := capabilityRegistry.EnsureAdminPermissions(ctx, permSvc); err != nil {
		pxlog.WarnF(ctx, "[capabilityRegistry] register permissions failed: %v", err)
	}

	capRegistryRepo := capabilityRegistryRepo.NewCapabilityRegistryRepository(db)
	bus := event_bus.NewLocalEventBus()
	capRegistrySvc := capabilityRegistry.NewService(capabilityRegistry.ServiceOptions{
		Repository:      capRegistryRepo,
		EventBus:        bus,
		Instrumentation: capabilityRegistryDomain.NewInstrumentation(nil),
		Auditor:         aud,
	})

	discoveryCacheStore := discoverycache.NewStore(cache.NewMemoryCache(), "")
	discoverySvc := discoveryService.NewService(discoveryService.ServiceOptions{
		RegistryRepository: capRegistryRepo,
		CacheStore:         discoveryCacheStore,
		Instrumentation:    capabilityRegistryDomain.NewInstrumentation(nil),
		DefaultTTL:         2 * time.Minute,
	})

	routerHealthRepo := capabilityHealth.NewMemoryRepository()
	routerSvc := capabilityRouter.NewService(capabilityRouter.ServiceOptions{
		RegistryRepository: capRegistryRepo,
		HealthRepository:   routerHealthRepo,
		EventBus:           bus,
		Instrumentation:    capabilityRegistryDomain.NewInstrumentation(nil),
	})
	sandboxSvc := capabilitySandbox.NewService(capRegistryRepo, routerSvc)

	eventFabricDeps := newEventFabricDeps(db, opts.EventFabric, bus)

	return &Deps{
		DB:                    db,
		TenantSvc:             tenantSvc,
		AuthUser:              authUser,
		AuthCustomer:          authCustomer,
		MeService:             meSvc,
		AuditSvc:              svc,
		Auditor:               aud,
		MediaMgr:              mediaManager,
		MediaSvc:              mediaSvc,
		EventBus:              bus,
		CapabilityRegistrySvc: capRegistrySvc,
		RouterSvc:             routerSvc,
		RouterSandboxSvc:      sandboxSvc,
		DiscoverySvc:          discoverySvc,
		EventFabric:           eventFabricDeps,
	}
}

// EventFabricDeps 聚合事件骨干运行时依赖。
type EventFabricDeps struct {
    RedisClient *redis.Client
    EventBus    event_bus.EventBus
    Repos       *EventFabricRepositoryFactory
    Config      EventFabricRuntimeConfig
    Directory   *directoryService.DirectoryService
}

// EventFabricRuntimeConfig 将配置项转换为运行时易用的结构。
type EventFabricRuntimeConfig struct {
	AckTimeout        time.Duration
	DefaultMaxRetry   int
	RetryKeyPrefix    string
	ReplayKeyPrefix   string
	SchedulerInterval time.Duration
}

// EventFabricRepositoryFactory 为事件骨干生成仓储实例的简单工厂。
type EventFabricRepositoryFactory struct {
	db *gorm.DB
}

func (f *EventFabricRepositoryFactory) WithDB(tx *gorm.DB) *EventFabricRepositoryFactory {
	if tx == nil {
		return f
	}
	return &EventFabricRepositoryFactory{db: tx}
}

func (f *EventFabricRepositoryFactory) TopicRepository() *eventfabricrepo.TopicRepository {
	if f == nil {
		return nil
	}
	return eventfabricrepo.NewTopicRepository(f.db)
}

func newEventFabricDeps(db *gorm.DB, opts EventFabricOptions, bus event_bus.EventBus) *EventFabricDeps {
	const (
		fallbackAckTimeout      = 30 * time.Second
		fallbackDefaultMaxRetry = 5
		fallbackSchedulerTick   = 5 * time.Second
	)

	cfg := EventFabricRuntimeConfig{
		AckTimeout:        time.Duration(opts.AckTimeoutSeconds) * time.Second,
		DefaultMaxRetry:   opts.DefaultMaxRetry,
		RetryKeyPrefix:    opts.RetryKeyPrefix,
		ReplayKeyPrefix:   opts.ReplayKeyPrefix,
		SchedulerInterval: time.Duration(opts.SchedulerInterval) * time.Second,
	}

	if cfg.AckTimeout <= 0 {
		cfg.AckTimeout = fallbackAckTimeout
	}
	if cfg.DefaultMaxRetry <= 0 {
		cfg.DefaultMaxRetry = fallbackDefaultMaxRetry
	}
	if strings.TrimSpace(cfg.RetryKeyPrefix) == "" {
		cfg.RetryKeyPrefix = "event_fabric:retry"
	}
	if strings.TrimSpace(cfg.ReplayKeyPrefix) == "" {
		cfg.ReplayKeyPrefix = "event_fabric:replay"
	}
	if cfg.SchedulerInterval <= 0 {
		cfg.SchedulerInterval = fallbackSchedulerTick
	}

	var redisClient *redis.Client
	if addr := strings.TrimSpace(opts.RedisAddr); addr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: opts.RedisPassword,
			DB:       opts.RedisDB,
		})
	}

	factory := &EventFabricRepositoryFactory{db: db}
	var directorySvc *directoryService.DirectoryService
	if repo := factory.TopicRepository(); repo != nil {
		directorySvc = directoryService.NewDirectoryService(directoryService.Options{
			Store:             repo,
			EventBus:          bus,
			Clock:             time.Now,
			ActorResolver:     func(context.Context) string { return "system" },
			DefaultMaxRetry:   cfg.DefaultMaxRetry,
			DefaultAckTimeout: cfg.AckTimeout,
		})
	}

	return &EventFabricDeps{
		RedisClient: redisClient,
		EventBus:    bus,
		Repos:       factory,
		Config:      cfg,
		Directory:   directorySvc,
	}
}
