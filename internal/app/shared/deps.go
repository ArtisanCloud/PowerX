package shared

// internal/bootstrap/deps.go

import (
	"context"
	"strings"
	"time"

	workers "github.com/ArtisanCloud/PowerX/internal/app/shared/workers"
	discoverycache "github.com/ArtisanCloud/PowerX/internal/infra/cache/discovery"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	discoveryService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/discovery"
	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	capabilityRegistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	capabilityRouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	capabilitySandbox "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/sandbox"
	aclService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	auditService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/audit"
	deliveryService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	directoryService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	dlqService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/dlq"
	eventmetrics "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/metrics"
	replayService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/replay"
	security "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/security"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	tenantsvc "github.com/ArtisanCloud/PowerX/internal/service/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
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
	sinks := []auditsvc.Sink{&auditsvc.LoggerSink{L: pxlog.GetGlobalLogger()}}
	svc := auditsvc.NewService(auditsvc.ServiceOptions{
		DB:     db,
		Sinks:  sinks,
		Config: opts.Audit,
	})
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

	bus := event_bus.NewLocalEventBus()
	capRegistrySvc := capabilityRegistry.NewService(capabilityRegistry.ServiceOptions{
		DB:              db,
		EventBus:        bus,
		Instrumentation: capabilityRegistryDomain.NewInstrumentation(nil),
		Auditor:         aud,
	})

	discoveryCacheStore := discoverycache.NewStore(cache.NewMemoryCache(), "")
	discoverySvc := discoveryService.NewService(discoveryService.ServiceOptions{
		DB:              db,
		CacheStore:      discoveryCacheStore,
		Instrumentation: capabilityRegistryDomain.NewInstrumentation(nil),
		DefaultTTL:      2 * time.Minute,
	})

	routerSvc := capabilityRouter.NewService(capabilityRouter.ServiceOptions{
		DB:              db,
		EventBus:        bus,
		Instrumentation: capabilityRegistryDomain.NewInstrumentation(nil),
	})
	sandboxSvc := capabilitySandbox.NewService(capabilitySandbox.ServiceOptions{
		DB:            db,
		RouterService: routerSvc,
	})

	eventFabricDeps := newEventFabricDeps(db, opts.EventFabric, bus, svc, tenantSvc)

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
	Config      EventFabricRuntimeConfig
	Directory   *directoryService.DirectoryService
	ACL         *aclService.ACLService
	Enforcer    *aclService.ACLEnforcer
	Reliable    event_bus.ReliableQueue
	Scheduler   *deliveryService.BackoffScheduler
	Delivery    deliveryService.Service
	DLQ         dlqService.Service
	Audit       auditService.Service
	Replay      *replayService.Service
	RetryWorker *workers.EventFabricRetryWorker
	Metrics     eventmetrics.Recorder
	Security    *security.Verifier
}

// EventFabricRuntimeConfig 将配置项转换为运行时易用的结构。
type EventFabricRuntimeConfig struct {
	AckTimeout        time.Duration
	DefaultMaxRetry   int
	RetryKeyPrefix    string
	ReplayKeyPrefix   string
	SchedulerInterval time.Duration
}

func newEventFabricDeps(db *gorm.DB, opts EventFabricOptions, bus event_bus.EventBus, auditSvc auditsvc.Service, tenantSvc *tenantsvc.TenantService) *EventFabricDeps {
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

	metricsRecorder := eventmetrics.NewRecorder()

	var securityVerifier *security.Verifier
	if opts.Security.RequireTLS || strings.TrimSpace(opts.Security.SignatureSecret) != "" {
		securityVerifier = security.NewVerifier(opts.Security)
	}

	var redisClient *redis.Client
	if addr := strings.TrimSpace(opts.RedisAddr); addr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: opts.RedisPassword,
			DB:       opts.RedisDB,
		})
	}

	var reliableQueue event_bus.ReliableQueue
	var scheduler *deliveryService.BackoffScheduler
	if redisClient != nil {
		reliableQueue = event_bus.NewRedisReliableQueue(redisClient)
		scheduler = deliveryService.NewBackoffScheduler(reliableQueue)
	}

	directorySvc := directoryService.NewDirectoryService(directoryService.Options{
		DB:                db,
		EventBus:          bus,
		Clock:             time.Now,
		ActorResolver:     func(context.Context) string { return "system" },
		DefaultMaxRetry:   cfg.DefaultMaxRetry,
		DefaultAckTimeout: cfg.AckTimeout,
	})

	aclSvc := aclService.NewACLService(aclService.Options{
		DB:    db,
		Clock: time.Now,
	})
	aclEnforcer := aclService.NewACLEnforcer(aclSvc)

	auditSvcEF := auditService.NewService(auditService.Options{
		AuditService: auditSvc,
		Source:       "event_fabric",
		ResourceType: "event",
		Clock:        time.Now,
	})

	var deliverySvc deliveryService.Service
	if scheduler != nil {
		var err error
		deliverySvc, err = deliveryService.NewService(deliveryService.Options{
			DB:        db,
			ACL:       aclSvc,
			Scheduler: scheduler,
			Clock:     time.Now,
			MaxRetry:  cfg.DefaultMaxRetry,
			Audit:     auditSvcEF,
			Metrics:   metricsRecorder,
		})
		if err != nil {
			pxlog.WarnF(context.Background(), "init delivery service failed: %v", err)
			deliverySvc = nil
		}
	}

	var dlqSvc dlqService.Service
	if deliverySvc != nil {
		var err error
		dlqSvc, err = dlqService.NewService(dlqService.Options{
			DB:       db,
			Delivery: deliverySvc,
			Audit:    auditSvcEF,
			Clock:    time.Now,
			Metrics:  metricsRecorder,
		})
		if err != nil {
			pxlog.WarnF(context.Background(), "init dlq service failed: %v", err)
			dlqSvc = nil
		}
	}

	var replaySvc *replayService.Service
	if deliverySvc != nil {
		replaySvc = replayService.NewService(replayService.Options{
			DB:       db,
			Delivery: deliverySvc,
			Clock:    time.Now,
			Metrics:  metricsRecorder,
		})
	}

	var retryWorker *workers.EventFabricRetryWorker
	if deliverySvc != nil && bus != nil {
		tenantProvider := func(ctx context.Context) ([]string, error) {
			if tenantSvc == nil {
				return []string{"global"}, nil
			}
			items, _, _, err := tenantSvc.List(ctx, tenantsvc.ListTenantsOption{Page: 1, PageSize: 1000})
			if err != nil {
				return nil, err
			}
			keys := make([]string, 0, len(items)+1)
			for _, item := range items {
				if key := strings.TrimSpace(item.Key); key != "" {
					keys = append(keys, key)
				}
			}
			keys = append(keys, "global")
			return keys, nil
		}
		retryWorker = workers.NewEventFabricRetryWorker(workers.EventFabricRetryWorkerOptions{
			Delivery:       deliverySvc,
			EventBus:       bus,
			TenantProvider: tenantProvider,
			Interval:       cfg.SchedulerInterval,
			BatchSize:      100,
		})
	}

	return &EventFabricDeps{
		RedisClient: redisClient,
		EventBus:    bus,
		Config:      cfg,
		Directory:   directorySvc,
		ACL:         aclSvc,
		Enforcer:    aclEnforcer,
		Reliable:    reliableQueue,
		Scheduler:   scheduler,
		Delivery:    deliverySvc,
		DLQ:         dlqSvc,
		Audit:       auditSvcEF,
		Replay:      replaySvc,
		RetryWorker: retryWorker,
		Metrics:     metricsRecorder,
		Security:    securityVerifier,
	}
}
