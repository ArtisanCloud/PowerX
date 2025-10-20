package shared

// internal/bootstrap/deps.go

import (
	"context"
	"fmt"
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
	authorizationService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	deliveryService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	directoryService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	dlqService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/dlq"
	eventmetrics "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/metrics"
	replayService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/replay"
	security "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/security"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	tenantsvc "github.com/ArtisanCloud/PowerX/internal/service/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type auditViolationReporter struct {
	audit auditService.Service
}

func (r auditViolationReporter) Report(ctx context.Context, violation security.Violation) {
	if r.audit == nil {
		return
	}
	if violation.Timestamp.IsZero() {
		violation.Timestamp = time.Now().UTC()
	}
	_ = r.audit.Write(ctx, auditService.Record{
		ID:          fmt.Sprintf("sandbox:%s:%s", violation.Type, violation.Method),
		TenantID:    "",
		Topic:       "event_fabric.authorization",
		PrincipalID: violation.Host,
		Action:      "SECURITY_SANDBOX_VIOLATION",
		Status:      "FAIL",
		Metadata: map[string]string{
			"detail": violation.Detail,
			"path":   violation.Path,
			"method": violation.Method,
			"host":   violation.Host,
		},
		HappenedAt: violation.Timestamp,
	})
}

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
	Workflow   *WorkflowDeps
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

	var (
		workflowReliable event_bus.ReliableQueue
		workflowScheduler *workflowsvc.Scheduler
	)
	if eventFabricDeps != nil {
		workflowReliable = eventFabricDeps.Reliable
		if workflowReliable != nil {
			workflowScheduler = workflowsvc.NewScheduler(workflowReliable)
		}
	}
	workflowSvc := workflowsvc.NewService(db, workflowsvc.ServiceOptions{})

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
		Workflow: &WorkflowDeps{
			Service:       workflowSvc,
			Scheduler:     workflowScheduler,
			ReliableQueue: workflowReliable,
		},
	}
}

// EventFabricDeps 聚合事件骨干运行时依赖。
type EventFabricDeps struct {
	RedisClient   *redis.Client
	EventBus      event_bus.EventBus
	Config        EventFabricRuntimeConfig
	Directory     *directoryService.DirectoryService
	ACL           *aclService.ACLService
	Enforcer      *aclService.ACLEnforcer
	Reliable      event_bus.ReliableQueue
	Scheduler     *deliveryService.BackoffScheduler
	Delivery      deliveryService.Service
	DLQ           dlqService.Service
	Audit         auditService.Service
	Replay        *replayService.Service
	RetryWorker   *workers.EventFabricRetryWorker
	Metrics       eventmetrics.Recorder
	Security      *security.Verifier
	Authorization *AuthorizationDeps
}

// WorkflowDeps 聚合工作流域运行时依赖。
type WorkflowDeps struct {
	Service      *workflowsvc.Service
	Scheduler    *workflowsvc.Scheduler
	ReliableQueue event_bus.ReliableQueue
}

// EventFabricRuntimeConfig 将配置项转换为运行时易用的结构。
type EventFabricRuntimeConfig struct {
	AckTimeout        time.Duration
	DefaultMaxRetry   int
	RetryKeyPrefix    string
	ReplayKeyPrefix   string
	SchedulerInterval time.Duration
}

// AuthorizationDeps 聚合授权域依赖。
type AuthorizationDeps struct {
	Service       authorizationService.Service
	Templates     authorizationService.TemplateService
	Cache         authorizationService.Cache
	Dispatcher    authorizationService.ChallengeDispatcher
	Secrets       *authorizationService.SecretsManager
	Limiter       authorizationService.RateLimiter
	Alerts        authorizationService.AlertEmitter
	Reporting     authorizationService.ReportingService
	TimeoutWorker *workers.EventFabricAuthorizationTimeoutWorker
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

	if securityVerifier != nil {
		securityVerifier.SetViolationReporter(auditViolationReporter{audit: auditSvcEF})
	}

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

	var authDeps *AuthorizationDeps
	{
		authRepo := eventfabricrepo.NewAuthorizationRepository(db)

		authRedis := redisClient
		authCfg := opts.Authorization
		if addr := strings.TrimSpace(authCfg.RedisAddr); addr != "" {
			needsNewClient := redisClient == nil ||
				addr != strings.TrimSpace(opts.RedisAddr) ||
				authCfg.RedisDB != opts.RedisDB ||
				authCfg.RedisPassword != opts.RedisPassword
			if needsNewClient {
				authRedis = redis.NewClient(&redis.Options{
					Addr:     addr,
					Password: authCfg.RedisPassword,
					DB:       authCfg.RedisDB,
				})
			}
		}

		cache := authorizationService.NewCache(authorizationService.CacheOptions{
			RedisClient:       authRedis,
			KeyPrefix:         "event_fabric:authorization",
			RedisTTL:          time.Duration(authCfg.CacheTTLSeconds) * time.Second,
			LocalTTL:          time.Duration(authCfg.LocalCacheTTLSeconds) * time.Second,
			LocalCapacity:     512,
			InvalidateChannel: authCfg.CacheInvalidateChannel,
			Logger:            pxlog.GetGlobalLogger(),
		})

		dispatcher := authorizationService.NewChallengeDispatcher(authorizationService.ChallengeDispatcherOptions{
			EventBus: bus,
			Topic:    authCfg.ChallengeTopic,
			Logger:   pxlog.GetGlobalLogger(),
		})

		alertEmitter := authorizationService.NewAlertEmitter(authorizationService.AlertEmitterOptions{
			EventBus: bus,
			Topic:    authCfg.AlertTopic,
			Logger:   pxlog.GetGlobalLogger(),
			Clock:    time.Now,
		})

		rateLimiter := authorizationService.NewRateLimiter(authorizationService.RateLimiterOptions{
			Client: authRedis,
			Prefix: authCfg.RateLimitPrefix,
			Logger: pxlog.GetGlobalLogger(),
			Clock:  time.Now,
		})

		var kmsClient authorizationService.KMSClient
		if strings.TrimSpace(authCfg.Secrets.Provider) != "" || strings.TrimSpace(authCfg.Secrets.KeyID) != "" {
			// 目前仅提供默认 Noop 客户端，后续可根据 Provider 扩展具体实现。
			kmsClient = authorizationService.NewNoopKMSClient()
		}
		secretsManager := authorizationService.NewSecretsManager(authorizationService.SecretsManagerOptions{
			Client:           kmsClient,
			KeyID:            authCfg.Secrets.KeyID,
			RotationInterval: time.Duration(authCfg.Secrets.RotationIntervalSeconds) * time.Second,
			CacheTTL:         time.Duration(authCfg.Secrets.CacheTTLSeconds) * time.Second,
			Logger:           pxlog.GetGlobalLogger(),
		})

		authService, err := authorizationService.NewService(authorizationService.ServiceOptions{
			Repository:   authRepo,
			Cache:        cache,
			Dispatcher:   dispatcher,
			Secrets:      secretsManager,
			ChallengeSLA: time.Duration(authCfg.ChallengeSLASeconds) * time.Second,
			Audit:        auditSvcEF,
			Metrics:      metricsRecorder,
			RateLimiter:  rateLimiter,
			Alerts:       alertEmitter,
			Logger:       pxlog.GetGlobalLogger(),
		})
		if err != nil {
			pxlog.WarnF(context.Background(), "init authorization service failed: %v", err)
			authService = nil
		}

		templateService := authorizationService.NewTemplateService(authRepo, time.Now)
		reportingService := authorizationService.NewReportingService(authorizationService.ReportingServiceOptions{
			AuditDB:                 db,
			AuthorizationRepository: authRepo,
			Logger:                  pxlog.GetGlobalLogger(),
		})

		var timeoutWorker *workers.EventFabricAuthorizationTimeoutWorker
		if authService != nil && tenantSvc != nil {
			tenantProvider := func(ctx context.Context) ([]uuid.UUID, error) {
				items, _, _, err := tenantSvc.List(ctx, tenantsvc.ListTenantsOption{Page: 1, PageSize: 1000})
				if err != nil {
					return nil, err
				}
				ids := make([]uuid.UUID, 0, len(items))
				for _, t := range items {
					if t.UUID != uuid.Nil {
						ids = append(ids, t.UUID)
					}
				}
				return ids, nil
			}
			timeoutWorker = workers.NewEventFabricAuthorizationTimeoutWorker(workers.EventFabricAuthorizationTimeoutWorkerOptions{
				Service:        authService,
				TenantProvider: tenantProvider,
				Interval:       time.Duration(authCfg.TimeoutSweepIntervalSeconds) * time.Second,
				Logger:         pxlog.GetGlobalLogger(),
			})
		}

		authDeps = &AuthorizationDeps{
			Service:       authService,
			Templates:     templateService,
			Cache:         cache,
			Dispatcher:    dispatcher,
			Secrets:       secretsManager,
			Limiter:       rateLimiter,
			Alerts:        alertEmitter,
			Reporting:     reportingService,
			TimeoutWorker: timeoutWorker,
		}
	}

	return &EventFabricDeps{
		RedisClient:   redisClient,
		EventBus:      bus,
		Config:        cfg,
		Directory:     directorySvc,
		ACL:           aclSvc,
		Enforcer:      aclEnforcer,
		Reliable:      reliableQueue,
		Scheduler:     scheduler,
		Delivery:      deliverySvc,
		DLQ:           dlqSvc,
		Audit:         auditSvcEF,
		Replay:        replaySvc,
		RetryWorker:   retryWorker,
		Metrics:       metricsRecorder,
		Security:      securityVerifier,
		Authorization: authDeps,
	}
}
