package shared

// internal/bootstrap/deps.go

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	toolstore "github.com/ArtisanCloud/PowerX/internal/agent/toolstore"
	workers "github.com/ArtisanCloud/PowerX/internal/app/shared/workers"
	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	discoverycache "github.com/ArtisanCloud/PowerX/internal/infra/cache/discovery"
	mediamgr "github.com/ArtisanCloud/PowerX/internal/infra/media/manager"
	imnotify "github.com/ArtisanCloud/PowerX/internal/notifications/im"
	capmetrics "github.com/ArtisanCloud/PowerX/internal/observability/metrics"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	igdeps "github.com/ArtisanCloud/PowerX/internal/server/mcp/tools/integration_gateway/deps"
	agentsettings "github.com/ArtisanCloud/PowerX/internal/service/agent"
	agentlifecycle "github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	agentinstr "github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle/instrumentation"
	authsvc "github.com/ArtisanCloud/PowerX/internal/service/auth"
	capabilitycatalog "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	discoveryService "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/discovery"
	capabilityRegistryDomain "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/domain"
	capabilityRegistry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	capabilityRouter "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	capabilitySandbox "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/sandbox"
	devhotloadservice "github.com/ArtisanCloud/PowerX/internal/service/dev_hotload"
	devhotloadinstrumentation "github.com/ArtisanCloud/PowerX/internal/service/dev_hotload/instrumentation"
	devhotloadstore "github.com/ArtisanCloud/PowerX/internal/service/dev_hotload/store"
	aclService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	auditService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/audit"
	authorizationService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/authorization"
	deliveryService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/delivery"
	directoryService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	dlqService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/dlq"
	manifestService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/manifest"
	eventmetrics "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/metrics"
	replayService "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/replay"
	cronschedulersvc "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/scheduler"
	security "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/security"
	eventshared "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	ticketbridge "github.com/ArtisanCloud/PowerX/internal/service/integration/ticketbridge"
	integrationgateway "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway"
	integrationInstrumentation "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/instrumentation"
	integrationManager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	integrationTenant "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/tenant"
	knowledgeService "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	kncompliance "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/compliance"
	knctxsnapshot "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/context_snapshot"
	decay_guard "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/decay_guard"
	ksdelta "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/delta"
	event_hotfix "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/event_hotfix"
	knowledgeinstr "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/instrumentation"
	knowledgeqa "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/qa_bridge"
	tenant_release "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/tenant_release"
	kntoolchain "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/toolchain"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	notificationssvc "github.com/ArtisanCloud/PowerX/internal/service/notifications"
	pluginbootstrap "github.com/ArtisanCloud/PowerX/internal/service/plugin_bootstrap"
	plugincompat "github.com/ArtisanCloud/PowerX/internal/service/plugin_compat"
	plugindiag "github.com/ArtisanCloud/PowerX/internal/service/plugin_debug/diagnostics"
	plugindebughost "github.com/ArtisanCloud/PowerX/internal/service/plugin_debug/host"
	plugingovernance "github.com/ArtisanCloud/PowerX/internal/service/plugin_governance"
	pluginimport "github.com/ArtisanCloud/PowerX/internal/service/plugin_import"
	pluginReleaseService "github.com/ArtisanCloud/PowerX/internal/service/plugin_release"
	pluginsandbox "github.com/ArtisanCloud/PowerX/internal/service/plugin_sandbox"
	tenantsvc "github.com/ArtisanCloud/PowerX/internal/service/tenant"
	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	wsbus "github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	knowledgeworkflow "github.com/ArtisanCloud/PowerX/internal/workflow/knowledge_space"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	auditsvc "github.com/ArtisanCloud/PowerX/pkg/corex/audit"
	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	integrationRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	compatrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_compat"
	plugindiagrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_debug"
	govrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_governance"
	pluginReleaseRepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_release"
	pluginsandboxrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/plugin_sandbox"
	vectorstorepkg "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	kafkadriver "github.com/ArtisanCloud/PowerX/pkg/event_bus/drivers/kafka"
	natsdriver "github.com/ArtisanCloud/PowerX/pkg/event_bus/drivers/nats"
	rabbitdriver "github.com/ArtisanCloud/PowerX/pkg/event_bus/drivers/rabbitmq"
	pxlog "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/internal/workflow"
	workflowengine "github.com/ArtisanCloud/PowerX/internal/workflow/engine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type auditViolationReporter struct {
	audit auditService.Service
}

type noopAuditRepository struct{}

func (noopAuditRepository) InsertBatch(context.Context, []dbm.AuditEvent) error { return nil }

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

	TenantSvc     *tenantsvc.TenantService
	MediaMgr      *mediamgr.MediaManager
	MediaSvc      *mediasvc.MediaService
	Notifications *notificationssvc.Service

	EventBus                          event_bus.EventBus
	CapabilityRegistrySvc             *capabilityRegistry.Service
	CapabilityCatalogSvc              *capabilitycatalog.RegistryService
	CapabilityRegistryAudit           *capabilitycatalog.AuditService
	CapabilityRegistryAlerts          capabilitycatalog.CapabilityAlerting
	CapabilityInvocationSvc           *capabilitycatalog.InvocationService
	CapabilityAuthorizer              *capabilitycatalog.AuthorizationService
	CapabilitySelector                *capabilitycatalog.Selector
	WorkflowCatalog                   *capabilitycatalog.WorkflowCatalog
	ToolStore                         *toolstore.Store
	VersionLockStore                  capabilitycatalog.VersionLock
	RouterSvc                         *capabilityRouter.Service
	RouterSandboxSvc                  *capabilitySandbox.Service
	DiscoverySvc                      *discoveryService.Service
	IntegrationGateway                *IntegrationGatewayDeps
	AgentLifecycle                    *AgentLifecycleDeps
	DevHotloadOptions                 DevHotloadOptions
	PluginReleaseOptions              PluginReleaseOptions
	PluginReleaseService              *pluginReleaseService.Service
	DevHotloadService                 *devhotloadservice.Service
	PluginBootstrapService            *pluginbootstrap.Service
	PluginImportService               *pluginimport.Service
	PluginDebugHost                   *plugindebughost.Service
	PluginDiagnostics                 *plugindiag.Service
	PluginSandbox                     *pluginsandbox.Service
	PluginGovernance                  *plugingovernance.Service
	PluginCompat                      *plugincompat.Service
	WorkflowTemplateSvc               *capabilitycatalog.WorkflowTemplateService
	WorkflowStepAdapter               *workflowengine.CapabilityStepAdapter
	CapabilityDefaultHTTPTimeout      time.Duration
	CapabilityAIMultimodalHTTPTimeout time.Duration

	EventFabric    *EventFabricDeps
	Workflow       *WorkflowDeps
	KnowledgeSpace *KnowledgeSpaceDeps
}

func NewDeps(db *gorm.DB, opts *DepsOptions) *Deps {
	ctx := context.Background()
	defaultHTTPTimeout := opts.CapabilityRegistry.DefaultHTTPTimeout
	if defaultHTTPTimeout <= 0 {
		defaultHTTPTimeout = 20 * time.Second
	}
	aiMultimodalHTTPTimeout := opts.CapabilityRegistry.AIMultimodalHTTPTimeout
	if aiMultimodalHTTPTimeout <= 0 {
		aiMultimodalHTTPTimeout = 5 * time.Minute
	}
	authUser := authsvc.NewAuthService(db, opts.AuthUser)
	authCustomer := authsvc.NewAuthService(db, opts.AuthCustomer)

	// --- Audit 初始化 ---
	sinks := []auditsvc.Sink{&auditsvc.LoggerSink{L: pxlog.GetGlobalLogger()}}
	if fileSink, err := auditsvc.NewFileSink(opts.AuditFileSink); err != nil {
		pxlog.WarnF(ctx, "[audit] init file sink failed: %v", err)
	} else if fileSink != nil {
		sinks = append(sinks, fileSink)
	}
	var auditRepo auditsvc.Repository
	if !opts.AuditPersistToDB {
		auditRepo = noopAuditRepository{}
	}
	svc := auditsvc.NewService(auditsvc.ServiceOptions{
		DB:         db,
		Repository: auditRepo,
		Sinks:      sinks,
		Config:     opts.Audit,
	})
	// 仅在显式开启时注册 GORM 行级审计回调。
	if opts.AuditEnableGORMCallbacks {
		auditsvc.RegisterAuditCallbacks(db, svc)
	}

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

	var capJobRepo *caprepo.CapabilitySyncJobRepository
	var capEventRepo *caprepo.CapabilityEventPublicationRepository
	var capTraceRepo *caprepo.InvocationTraceRepository
	if db != nil {
		capJobRepo = caprepo.NewCapabilitySyncJobRepository(db)
		capEventRepo = caprepo.NewCapabilityEventPublicationRepository(db)
		capTraceRepo = caprepo.NewInvocationTraceRepository(db)
	}

	capMetrics := capmetrics.NewCapabilityRegistryMetrics(nil)

	capAuditSvc := capabilitycatalog.NewAuditService(capabilitycatalog.AuditServiceOptions{
		JobRepo:   capJobRepo,
		EventRepo: capEventRepo,
		TraceRepo: capTraceRepo,
		EventBus:  bus,
		Auditor:   aud,
		Metrics:   capMetrics,
		Clock:     time.Now,
	})

	var capNotifier capabilitycatalog.NotificationSender
	if strings.TrimSpace(opts.CapabilityRegistry.Notifications.IMWebhook) != "" {
		capNotifier = imnotify.NewSender(imnotify.Config{
			WebhookURL:    opts.CapabilityRegistry.Notifications.IMWebhook,
			RetryInterval: opts.CapabilityRegistry.Notifications.RetryInterval,
			MaxRetry:      opts.CapabilityRegistry.Notifications.RetryMaxAttempts,
			HTTPTimeout:   opts.CapabilityRegistry.Notifications.HTTPTimeout,
		})
	}
	capAlerting := capabilitycatalog.NewAlertingService(capabilitycatalog.AlertingOptions{
		Audit:    svc,
		Notifier: capNotifier,
		Logger:   pxlog.GetGlobalLogger(),
		Clock:    time.Now,
	})

	discoveryCacheStore := discoverycache.NewStore(cache.NewMemoryCache(), "")
	var registryRepo *caprepo.CapabilityRegistryRepository
	if db != nil {
		registryRepo = caprepo.NewCapabilityRegistryRepository(db)
	}

	discoverySvc := discoveryService.NewService(discoveryService.ServiceOptions{
		DB:              db,
		CacheStore:      discoveryCacheStore,
		Instrumentation: capabilityRegistryDomain.NewInstrumentation(nil),
		DefaultTTL:      2 * time.Minute,
	})

	routerSvc := capabilityRouter.NewService(capabilityRouter.ServiceOptions{
		DB:                 db,
		RegistryRepository: registryRepo,
		EventBus:           bus,
		Instrumentation:    capabilityRegistryDomain.NewInstrumentation(nil),
	})
	sandboxSvc := capabilitySandbox.NewService(capabilitySandbox.ServiceOptions{
		DB:            db,
		RouterService: routerSvc,
	})

	eventFabricDeps := newEventFabricDeps(db, opts.EventFabric, opts.Queue, bus, svc, tenantSvc)

	var (
		workflowReliable  event_bus.ReliableQueue
		workflowScheduler *workflowsvc.Scheduler
	)
	if eventFabricDeps != nil {
		workflowReliable = eventFabricDeps.Reliable
		if workflowReliable != nil {
			workflowScheduler = workflowsvc.NewScheduler(workflowReliable)
		}
	}
	workflowSvc := workflowsvc.NewService(db, workflowsvc.ServiceOptions{
		ReliableQueue: workflowReliable,
		Scheduler:     workflowScheduler,
	})

	integrationGatewayDeps := newIntegrationGatewayDeps(db, opts.IntegrationGateway, bus, aud)

	var versionLockRedis redis.UniversalClient
	if integrationGatewayDeps != nil {
		versionLockRedis = integrationGatewayDeps.RedisClient
	}
	versionLockStore := toolstore.NewVersionLockStore(toolstore.VersionLockStoreOptions{
		Redis:    versionLockRedis,
		EventBus: bus,
		Clock:    time.Now,
	})

	var capabilityCatalogSvc *capabilitycatalog.RegistryService
	var capabilityInvocationSvc *capabilitycatalog.InvocationService
	var capabilityAuthorizer *capabilitycatalog.AuthorizationService
	var capabilitySelector *capabilitycatalog.Selector
	var workflowCatalog *capabilitycatalog.WorkflowCatalog
	var workflowTemplateSvc *capabilitycatalog.WorkflowTemplateService
	var workflowStepAdapter *workflowengine.CapabilityStepAdapter
	var workflowTelemetry *workflowengine.WorkflowTelemetry
	var toolStore *toolstore.Store
	if db != nil {
		var redisClient redis.UniversalClient
		if integrationGatewayDeps != nil {
			redisClient = integrationGatewayDeps.RedisClient
		}
		workflowTemplateRepo := caprepo.NewWorkflowTemplateRepository(db)
		recordRepo := caprepo.NewCapabilityRecordRepository(db, redisClient)
		capabilityCatalogSvc = capabilitycatalog.NewRegistryService(capabilitycatalog.RegistryServiceOptions{
			DB:           db,
			Redis:        redisClient,
			TemplateRepo: workflowTemplateRepo,
		})

		baseCapabilitySeeder := integrationgateway.NewBaseCapabilitySeeder(integrationgateway.BaseCapabilitySeederOptions{
			RecordRepo:   recordRepo,
			RegistryRepo: registryRepo,
			TenantRepo:   tenantSvc.Repo,
			Logger:       pxlog.GetGlobalLogger(),
			Clock:        time.Now,
		})
		if err := baseCapabilitySeeder.Ensure(ctx); err != nil {
			pxlog.WarnF(ctx, "[integration_gateway] seed platform capabilities failed: %v", err)
		}

		policyGenerator := capabilitycatalog.NewPolicyGenerator(capabilitycatalog.PolicyGeneratorOptions{
			RecordRepo: recordRepo,
			Cache: capabilitycatalog.NewCacheManager(capabilitycatalog.CacheManagerOptions{
				Redis: redisClient,
			}),
			Clock: time.Now,
		})
		toolStore = toolstore.NewStore(toolstore.StoreOptions{
			Generator: policyGenerator,
			EventBus:  bus,
			Logger:    pxlog.GetGlobalLogger(),
			Clock:     time.Now,
		})
		toolstore.SetGlobalStore(toolStore)

		httpBaseURL := strings.TrimSpace(os.Getenv("POWERX_HTTP_PROXY_BASE"))
		if httpBaseURL == "" {
			httpPort := 8080
			if opts.Server.HTTPPort > 0 {
				httpPort = opts.Server.HTTPPort
			}
			httpBaseURL = fmt.Sprintf("http://127.0.0.1:%d", httpPort)
		}
		httpProxyClient := &http.Client{
			Timeout: defaultHTTPTimeout,
		}
		aiModalHTTPProxyClient := &http.Client{
			Timeout: aiMultimodalHTTPTimeout,
		}
		var invocationGRPCConn *grpc.ClientConn
		grpcTarget := strings.TrimSpace(os.Getenv("POWERX_GRPC_PROXY_ADDR"))
		if grpcTarget == "" {
			grpcHost := strings.TrimSpace(opts.Server.GRPC.Host)
			if grpcHost == "" || grpcHost == "0.0.0.0" || grpcHost == "::" {
				grpcHost = "127.0.0.1"
			}
			grpcPort := opts.Server.GRPC.Port
			if grpcPort == 0 {
				grpcPort = 9001
			}
			grpcTarget = fmt.Sprintf("%s:%d", grpcHost, grpcPort)
		}
		if grpcTarget != "" {
			conn, err := grpc.DialContext(ctx, grpcTarget, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				pxlog.WarnF(ctx, "[capability_invocation] dial grpc proxy failed: %v", err)
			} else {
				invocationGRPCConn = conn
			}
		}

		capabilityInvocationSvc = capabilitycatalog.NewInvocationService(capabilitycatalog.InvocationServiceOptions{
			Catalog:           capabilityCatalogSvc,
			Router:            routerSvc,
			TraceRepo:         capTraceRepo,
			EventRepo:         capEventRepo,
			EventBus:          bus,
			Auditor:           aud,
			Audit:             capAuditSvc,
			Clock:             time.Now,
			VersionLock:       versionLockStore,
			HTTPClient:        httpProxyClient,
			AIModalHTTPClient: aiModalHTTPProxyClient,
			HTTPBaseURL:       httpBaseURL,
			GRPCConn:          invocationGRPCConn,
			ModelVerifier:     capabilitycatalog.NewTenantModelKeyVerifier(db),
		})
		var snapshotProvider capabilitycatalog.SnapshotProviderFunc
		if toolStore != nil {
			snapshotProvider = capabilitycatalog.SnapshotProviderFunc(func(ctx context.Context, tenant string, grants []string) (capabilitycatalog.SelectorPolicySnapshot, error) {
				snap, err := toolStore.GetSnapshot(ctx, tenant, grants)
				if err != nil {
					return capabilitycatalog.SelectorPolicySnapshot{}, err
				}
				return snap.ToRegistrySnapshot(), nil
			})
		}
		var safeModeStore capabilitycatalog.SafeModeStore
		if redisClient != nil {
			safeModeStore = capabilitycatalog.NewRedisSafeModeStore(capabilitycatalog.SafeModeStoreOptions{
				Redis: redisClient,
			})
		}
		capabilityAuthorizer = capabilitycatalog.NewAuthorizationService(capabilitycatalog.AuthorizationOptions{
			Catalog:  capabilityCatalogSvc,
			SafeMode: safeModeStore,
		})
		workflowTelemetry = workflowengine.NewWorkflowTelemetry(capMetrics)

		capabilitySelector = capabilitycatalog.NewSelector(capabilitycatalog.SelectorOptions{
			Store:      snapshotProvider,
			Invoker:    capabilityInvocationSvc,
			EventBus:   bus,
			Metrics:    capMetrics,
			Authorizer: capabilityAuthorizer,
		})

		if capabilitySelector != nil {
			workflowStepAdapter = workflowengine.NewCapabilityStepAdapter(capabilitySelector, workflowTelemetry)
		}

		if registry, err := toolstore.NewMCPRegistry(toolstore.MCPRegistryOptions{
			Catalog:     capabilityCatalogSvc,
			Invoker:     capabilityInvocationSvc,
			Clock:       time.Now,
			VersionLock: versionLockStore,
		}); err != nil {
			pxlog.WarnF(ctx, "[mcp] initialize registry failed: %v", err)
		} else {
			toolstore.SetGlobalMCPRegistry(registry)
		}

		workflowCatalog = capabilitycatalog.NewWorkflowCatalog(capabilitycatalog.WorkflowCatalogOptions{
			TemplateRepo: workflowTemplateRepo,
			RecordRepo:   recordRepo,
			Redis:        redisClient,
			Clock:        time.Now,
			Telemetry:    workflowTelemetry,
		})

		workflowTemplateSvc = capabilitycatalog.NewWorkflowTemplateService(capabilitycatalog.WorkflowTemplateServiceOptions{
			TemplateRepo: workflowTemplateRepo,
			ApprovalRepo: caprepo.NewWorkflowTemplateApprovalRepository(db),
			Clock:        time.Now,
		})
	}

	agentLifecycleDeps := newAgentLifecycleDeps(db, opts.AgentLifecycle, bus, svc)
	knowledgeDeps := newKnowledgeSpaceDeps(db, opts.KnowledgeSpace, bus, svc, eventFabricDeps)

	pluginReleaseCandidateRepo := pluginReleaseRepo.NewReleaseCandidateRepository(db)
	pluginReleasePlanRepo := pluginReleaseRepo.NewReleasePlanRepository(db)
	pluginReleaseDistributionRepo := pluginReleaseRepo.NewDistributionRepository(db)
	pluginReleaseSessionRepo := pluginReleaseRepo.NewLocalInstallSessionRepository(db)
	pluginDebugReportRepo := plugindiagrepo.NewReportRepository(db)
	pluginSandboxRunRepo := pluginsandboxrepo.NewRunRepository(db)
	pluginGovernanceRepo := govrepo.NewReportRepository(db)
	pluginCompatRepo := compatrepo.NewExceptionRepository(db)
	pluginImportRepo := pluginReleaseRepo.NewImportRepository(db)
	componentName := strings.TrimSpace(opts.PluginRelease.Observability.AlertRulePrefix)
	if componentName == "" {
		componentName = "powerx.plugin_release"
	}
	pluginReleaseSvc := pluginReleaseService.NewService(
		pluginReleaseCandidateRepo,
		pluginReleasePlanRepo,
		pluginReleaseDistributionRepo,
		pluginReleaseSessionRepo,
		componentName,
		pluginReleaseService.Options{
			FeatureFlags: pluginReleaseService.FeatureFlagOptions{
				EnableLocalInstall: opts.PluginRelease.FeatureFlags.EnableLocalInstall,
			},
			LocalInstall: pluginReleaseService.LocalInstallOptions{
				SessionTTL:        opts.PluginRelease.LocalInstall.SessionTTL,
				MaxArtifactSizeMB: opts.PluginRelease.LocalInstall.MaxArtifactSizeMB,
			},
			Auditor:  aud,
			EventBus: bus,
			Runtime: pluginReleaseService.RuntimeOptions{
				RollbackTimeout: opts.PluginRelease.Canary.RollbackTimeout,
			},
			Distribution: pluginReleaseService.DistributionOptions{
				OfflineBucket:       opts.PluginRelease.Distribution.OfflineBucket,
				OfflinePrefix:       opts.PluginRelease.Distribution.OfflinePrefix,
				EscalationThreshold: opts.PluginRelease.Distribution.EscalationThreshold,
				ArtifactRetention:   opts.PluginRelease.Distribution.ArtifactRetention,
				ReviewSLA:           48 * time.Hour,
			},
		},
	)

	var pluginBootstrapSvc *pluginbootstrap.Service
	if opts.PluginBootstrap.TemplatesPath != "" {
		var err error
		pluginBootstrapSvc, err = pluginbootstrap.NewService(pluginbootstrap.Options{
			TemplatesPath:   opts.PluginBootstrap.TemplatesPath,
			DefaultTemplate: opts.PluginBootstrap.DefaultTemplate,
			AllowHosts:      opts.PluginBootstrap.AllowHosts,
			Auditor:         aud,
			AuditSvc:        svc,
			Now:             time.Now,
		})
		if err != nil {
			pxlog.WarnF(ctx, "[plugin_bootstrap] initialize failed: %v", err)
		}
	}

	pluginImportSvc := pluginimport.NewService(pluginimport.Options{
		Repo:     pluginImportRepo,
		Auditor:  aud,
		AuditSvc: svc,
		Now:      time.Now,
	})

	var devHotloadSvc *devhotloadservice.Service
	devHotloadOpts := convertDevHotloadOptions(opts.DevHotload)
	if devHotloadOpts.FeatureFlags.Enabled {
		devStore := devhotloadstore.New(db, time.Now)
		registry := devhotloadservice.NewRegistry(devStore, nil, devhotloadservice.RegistryOptions{
			TTL:             devHotloadOpts.Sessions.TTL,
			MaxConcurrent:   devHotloadOpts.Sessions.MaxConcurrent,
			CleanupInterval: devHotloadOpts.Sessions.CleanupInterval,
			KeyPrefix:       "devhotload",
			Security:        devHotloadOpts.Security,
		})
		metrics := devhotloadinstrumentation.New(devHotloadOpts.Observability.MetricsNamespace)
		devHotloadSvc = devhotloadservice.NewService(devhotloadservice.ServiceDeps{
			Store:    devStore,
			Registry: registry,
			Auditor:  aud,
			Options:  devHotloadOpts,
			Metrics:  metrics,
			Notifier: devhotloadservice.NewNotifier(0),
		})
	}

	tenantConfig := integrationTenant.Config{
		DefaultRateLimit: integrationManager.RateLimitPolicy{
			Limit:         opts.IntegrationGateway.DefaultRateLimit.Limit,
			Burst:         opts.IntegrationGateway.DefaultRateLimit.Burst,
			WindowSeconds: opts.IntegrationGateway.DefaultRateLimit.WindowSeconds,
			Scope:         opts.IntegrationGateway.DefaultRateLimit.Scope,
		},
		EventTopics: integrationManager.EventTopics(opts.IntegrationGateway.EventTopics),
	}
	integrationGatewayDeps.Tenant = integrationTenant.NewService(integrationTenant.ServiceOptions{
		DB:              db,
		Router:          routerSvc,
		RateLimiter:     integrationGatewayDeps.RateLimiter,
		EventBus:        bus,
		Instrumentation: integrationGatewayDeps.Instrumentation,
		Auditor:         aud,
		Config:          tenantConfig,
		Clock:           time.Now,
	})

	if err := igdeps.Set(igdeps.ToolDependencies{
		TenantService:   integrationGatewayDeps.Tenant,
		ManagerService:  integrationGatewayDeps.Manager,
		Instrumentation: integrationGatewayDeps.Instrumentation,
	}); err != nil {
		pxlog.WarnF(ctx, "[integrationGateway] set MCP tool deps failed: %v", err)
	}

	ticketBridgeSvc := ticketbridge.NewService(ticketbridge.Options{
		Provider: opts.PluginDebug.TicketBridge.Provider,
		Endpoint: opts.PluginDebug.TicketBridge.Endpoint,
		Project:  opts.PluginDebug.TicketBridge.Project,
	})

	var pluginDebugHostSvc *plugindebughost.Service
	if opts.PluginDebug.HostSimulator.Enabled && featureFlagEnabled(opts.PluginDebug.HostSimulator.FeatureFlag) {
		component := strings.TrimSpace(opts.PluginDebug.Component)
		if component == "" {
			component = "plugin_debug"
		}
		pluginDebugHostSvc = plugindebughost.NewService(svc, plugindebughost.Options{
			Component:     component,
			ConfigPath:    opts.PluginDebug.HostSimulator.ConfigPath,
			PruneInterval: time.Minute,
			Now:           time.Now,
		})
	}
	var pluginDiagnosticsSvc *plugindiag.Service
	if pluginDebugReportRepo != nil {
		template, err := plugindiag.LoadTemplate(opts.PluginDebug.Reports.TemplatePath)
		if err != nil {
			pxlog.WarnF(ctx, "[plugin_debug] load report template failed: %v", err)
		}
		masker, err := plugindiag.LoadMasker(opts.PluginDebug.Reports.MaskingRulesPath)
		if err != nil {
			pxlog.WarnF(ctx, "[plugin_debug] load masking rules failed: %v", err)
		}
		pluginDiagnosticsSvc = plugindiag.NewService(pluginDebugReportRepo, svc, time.Now, plugindiag.Options{
			Template:        template,
			Masker:          masker,
			TicketBridge:    ticketBridgeSvc,
			FallbackLogBase: opts.PluginDebug.Reports.FallbackLogBase,
		})
	}

	var pluginSandboxSvc *pluginsandbox.Service
	if pluginSandboxRunRepo != nil && opts.PluginDebug.Sandbox.Enabled && featureFlagEnabled(opts.PluginDebug.Sandbox.FeatureFlag) {
		suite, err := pluginsandbox.LoadSuite(opts.PluginDebug.Sandbox.DataSuitePath)
		if err != nil {
			pxlog.WarnF(ctx, "[plugin_sandbox] load data suite failed: %v", err)
		}
		pluginSandboxSvc = pluginsandbox.NewService(pluginSandboxRunRepo, pluginsandbox.Options{
			Suite: suite,
			Now:   time.Now,
		})
	}

	var pluginGovernanceSvc *plugingovernance.Service
	if pluginGovernanceRepo != nil {
		pluginGovernanceSvc = plugingovernance.NewService(pluginGovernanceRepo, pluginReleaseCandidateRepo, time.Now)
	}

	var pluginCompatSvc *plugincompat.Service
	if pluginCompatRepo != nil {
		pluginCompatSvc = plugincompat.NewService(pluginCompatRepo, time.Now)
	}

	return &Deps{
		DB:                                db,
		TenantSvc:                         tenantSvc,
		AuthUser:                          authUser,
		AuthCustomer:                      authCustomer,
		MeService:                         meSvc,
		AuditSvc:                          svc,
		Auditor:                           aud,
		MediaMgr:                          mediaManager,
		MediaSvc:                          mediaSvc,
		Notifications:                     notificationssvc.NewService(db),
		EventBus:                          bus,
		CapabilityRegistrySvc:             capRegistrySvc,
		CapabilityCatalogSvc:              capabilityCatalogSvc,
		CapabilityRegistryAudit:           capAuditSvc,
		CapabilityRegistryAlerts:          capAlerting,
		CapabilityInvocationSvc:           capabilityInvocationSvc,
		CapabilityAuthorizer:              capabilityAuthorizer,
		CapabilitySelector:                capabilitySelector,
		WorkflowCatalog:                   workflowCatalog,
		WorkflowTemplateSvc:               workflowTemplateSvc,
		ToolStore:                         toolStore,
		VersionLockStore:                  versionLockStore,
		RouterSvc:                         routerSvc,
		RouterSandboxSvc:                  sandboxSvc,
		DiscoverySvc:                      discoverySvc,
		IntegrationGateway:                integrationGatewayDeps,
		AgentLifecycle:                    agentLifecycleDeps,
		KnowledgeSpace:                    knowledgeDeps,
		DevHotloadOptions:                 opts.DevHotload,
		PluginReleaseOptions:              opts.PluginRelease,
		PluginReleaseService:              pluginReleaseSvc,
		DevHotloadService:                 devHotloadSvc,
		PluginBootstrapService:            pluginBootstrapSvc,
		PluginImportService:               pluginImportSvc,
		PluginDebugHost:                   pluginDebugHostSvc,
		PluginDiagnostics:                 pluginDiagnosticsSvc,
		PluginSandbox:                     pluginSandboxSvc,
		PluginGovernance:                  pluginGovernanceSvc,
		PluginCompat:                      pluginCompatSvc,
		WorkflowStepAdapter:               workflowStepAdapter,
		CapabilityDefaultHTTPTimeout:      defaultHTTPTimeout,
		CapabilityAIMultimodalHTTPTimeout: aiMultimodalHTTPTimeout,
		EventFabric:                       eventFabricDeps,
		Workflow: &WorkflowDeps{
			Service:       workflowSvc,
			Scheduler:     workflowScheduler,
			ReliableQueue: workflowReliable,
		},
	}
}

func featureFlagEnabled(flag string) bool {
	flag = strings.TrimSpace(flag)
	if flag == "" {
		return true
	}
	value := strings.TrimSpace(os.Getenv(flag))
	if value == "" {
		return true
	}
	value = strings.ToLower(value)
	return value == "1" || value == "true" || value == "enabled" || value == "on" || value == "yes"
}

func convertDevHotloadOptions(src DevHotloadOptions) devhotloadservice.Options {
	return devhotloadservice.Options{
		FeatureFlags: devhotloadservice.FeatureFlags{
			Enabled:          src.FeatureFlags.Enabled,
			GatewayFlag:      src.FeatureFlags.GatewayFlag,
			SessionAuditFlag: src.FeatureFlags.SessionAuditFlag,
		},
		Sessions: devhotloadservice.SessionOptions{
			TTL:             src.Sessions.TTL,
			MaxConcurrent:   src.Sessions.MaxConcurrent,
			CleanupInterval: src.Sessions.CleanupInterval,
		},
		Sandbox: devhotloadservice.SandboxOptions{
			Image:          src.Sandbox.Image,
			MaxCPUPercent:  src.Sandbox.MaxCPUPercent,
			MaxMemoryMB:    src.Sandbox.MaxMemoryMB,
			WatchFileLimit: src.Sandbox.WatchFileLimit,
		},
		Security: devhotloadservice.SecurityOptions{
			RequireMTLS:     src.Security.RequireMTLS,
			AllowedSubjects: src.Security.AllowedSubjects,
			PATHeader:       src.Security.PATHeader,
			TokenTTL:        src.Security.TokenTTL,
			TokenSecret:     src.Security.TokenSecret,
			TokenIssuer:     src.Security.TokenIssuer,
			TokenAudience:   src.Security.TokenAudience,
			TokenPlatforms:  append([]string{}, src.Security.TokenPlatforms...),
			TokenRoles:      append([]string{}, src.Security.TokenRoles...),
			ImpersonateRoot: src.Security.ImpersonateRoot,
		},
		Observability: devhotloadservice.ObservabilityOptions{
			MetricsNamespace: src.Observability.MetricsNamespace,
			SSEBufferSize:    src.Observability.SSEBufferSize,
			AuditTopic:       src.Observability.AuditTopic,
		},
	}
}

// EventFabricDeps 聚合事件骨干运行时依赖。
type EventFabricDeps struct {
	RedisClient          *redis.Client
	EventBus             event_bus.EventBus
	Config               EventFabricRuntimeConfig
	Directory            *directoryService.DirectoryService
	ACL                  *aclService.ACLService
	Enforcer             *aclService.ACLEnforcer
	Seeder               *manifestService.SeedService
	BindingStore         manifestService.BindingStore
	Reliable             event_bus.ReliableQueue
	TaskDriver           event_bus.TaskDriver
	Scheduler            *deliveryService.BackoffScheduler
	Delivery             deliveryService.Service
	DLQ                  dlqService.Service
	Audit                auditService.Service
	Replay               *replayService.Service
	RetryWorker          *workers.EventFabricRetryWorker
	CronDispatcherWorker *workers.EventFabricCronDispatcherWorker
	NotificationWorker   *workers.EventFabricSystemNotificationDispatchWorker
	Metrics              eventmetrics.Recorder
	Security             *security.Verifier
	Authorization        *AuthorizationDeps
}

// WorkflowDeps 聚合工作流域运行时依赖。
type WorkflowDeps struct {
	Service       *workflowsvc.Service
	Scheduler     *workflowsvc.Scheduler
	ReliableQueue event_bus.ReliableQueue
}

// IntegrationGatewayDeps 聚合集成网关运行时所需依赖。
type IntegrationGatewayDeps struct {
	RateLimiter     authorizationService.RateLimiter
	Config          IntegrationGatewayRuntimeConfig
	RedisClient     *redis.Client
	Manager         *integrationManager.Service
	Tenant          *integrationTenant.Service
	Instrumentation *integrationInstrumentation.Instrumentation
}

// KnowledgeSpaceDeps 聚合知识空间域运行依赖。
type KnowledgeSpaceDeps struct {
	Instrumentation *knowledgeinstr.Instrumentation
	RedisClient     *redis.Client
	EventBus        event_bus.EventBus
	Config          KnowledgeSpaceRuntimeConfig
	Service         *knowledgeService.Service
	Ingestion       *knowledgeService.IngestionService
	Fusion          *knowledgeService.FusionService
	Feedback        *knowledgeService.FeedbackService
	CorpusCheck     *knowledgeService.CorpusCheckService
	Delta           *ksdelta.Service
	EventHotfix     *event_hotfix.Service
	DecayGuard      *decay_guard.Service
	Release         *tenant_release.Service
	VectorStore     vectorstorepkg.Store
	QABridge        *knowledgeqa.Service
}

// KnowledgeSpaceRuntimeConfig 描述运行期常用配置。
type KnowledgeSpaceRuntimeConfig struct {
	LockKeyPrefix            string
	MetricsKeyPrefix         string
	DefaultRetentionMonths   int
	ProvisioningSLA          time.Duration
	IngestionSLA             time.Duration
	SceneStrategyCatalogPath string
	EventTopics              KnowledgeSpaceEventTopicsOptions
	Notifications            KnowledgeSpaceNotificationOptions
}

// AgentLifecycleDeps 聚合 Agent 生命周期运行所需依赖。
type AgentLifecycleDeps struct {
	ProfileRepo      *agentrepo.AgentProfileLifecycleRepository
	LifecycleRepo    *agentrepo.AgentLifecycleEventRepository
	HealthRepo       *agentrepo.AgentHealthSnapshotRepository
	ShareRepo        *agentrepo.AgentShareRepository
	TenantFormRepo   *agentrepo.AgentTenantFormRepository
	Instrumentation  *agentinstr.Instrumentation
	Notifications    agentlifecycle.Notifier
	RedisClient      *redis.Client
	EventBus         event_bus.EventBus
	Config           AgentLifecycleRuntimeConfig
	Service          *agentlifecycle.Service
	PolicyEngine     agentlifecycle.PolicyConflictEngine
	ApprovalFlow     agentlifecycle.ApprovalFlow
	ShareValidator   agentlifecycle.ShareValidator
	QuotaProvisioner agentlifecycle.QuotaProvisioner
}

// AgentLifecycleRuntimeConfig 提供运行时常用配置。
type AgentLifecycleRuntimeConfig struct {
	CapacityKeyPrefix        string
	HealthKeyPrefix          string
	DefaultCapacityInstances int
	EventTopics              AgentLifecycleEventTopicsOptions
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
	Service           authorizationService.Service
	Templates         authorizationService.TemplateService
	Cache             authorizationService.Cache
	Dispatcher        authorizationService.ChallengeDispatcher
	Secrets           *authorizationService.SecretsManager
	Limiter           authorizationService.RateLimiter
	Alerts            authorizationService.AlertEmitter
	Reporting         authorizationService.ReportingService
	TimeoutTaskWorker *workers.EventFabricAuthorizationTimeoutTaskWorker
}

// IntegrationGatewayRuntimeConfig 简化运行时常用参数。
type IntegrationGatewayRuntimeConfig struct {
	RateLimitPrefix string
	DefaultPolicy   authorizationService.RateLimitPolicy
	EventTopics     IntegrationGatewayEventTopicsOptions
}

func newEventFabricDeps(db *gorm.DB, opts EventFabricOptions, queueOpts QueueOptions, bus event_bus.EventBus, auditSvc auditsvc.Service, tenantSvc *tenantsvc.TenantService) *EventFabricDeps {
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
	var taskDriver event_bus.TaskDriver
	var scheduler *deliveryService.BackoffScheduler
	retryWorkerFallbackEnabled := false
	retryWorkerDriverName := ""
	if redisClient != nil {
		reliableQueue = event_bus.NewRedisReliableQueue(redisClient)
		redisTaskDriver := event_bus.NewRedisTaskDriver(event_bus.RedisTaskDriverOptions{
			Client:           redisClient,
			Prefix:           "event_fabric:task",
			BlockingTimeout:  cfg.SchedulerInterval,
			ProcessingExpiry: cfg.AckTimeout * 2,
		})
		taskDriver = redisTaskDriver
		retryWorkerDriverName = string(taskDriver.Type())
		retryWorkerFallbackEnabled = true
		switch strings.ToLower(strings.TrimSpace(queueOpts.Driver)) {
		case "kafka":
			taskDriver = kafkadriver.NewDriver(kafkadriver.DriverOptions{
				Brokers:        append([]string{}, queueOpts.Kafka.Brokers...),
				TopicPrefix:    strings.TrimSpace(queueOpts.Kafka.TopicPrefix),
				ConsumerGroup:  strings.TrimSpace(queueOpts.Kafka.ConsumerGroup),
				PollTimeout:    time.Duration(queueOpts.Kafka.PollTimeoutMs) * time.Millisecond,
				FallbackDriver: redisTaskDriver,
			})
			pxlog.WarnF(context.Background(), "[event_fabric.task_driver] queue.driver=kafka enabled; using kafka adapter with redis fallback")
			retryWorkerDriverName = string(taskDriver.Type())
		case "rabbitmq":
			taskDriver = rabbitdriver.NewDriver(rabbitdriver.DriverOptions{
				URL:            strings.TrimSpace(queueOpts.Rabbit.URL),
				Exchange:       strings.TrimSpace(queueOpts.Rabbit.Exchange),
				QueuePrefix:    strings.TrimSpace(queueOpts.Rabbit.QueuePrefix),
				ConsumerTag:    strings.TrimSpace(queueOpts.Rabbit.ConsumerTag),
				Prefetch:       queueOpts.Rabbit.Prefetch,
				PollTimeout:    time.Duration(queueOpts.Rabbit.PollTimeoutMs) * time.Millisecond,
				FallbackDriver: redisTaskDriver,
			})
			pxlog.WarnF(context.Background(), "[event_fabric.task_driver] queue.driver=rabbitmq enabled; using rabbitmq adapter with redis fallback")
			retryWorkerDriverName = string(taskDriver.Type())
		case "nats":
			taskDriver = natsdriver.NewDriver(natsdriver.DriverOptions{
				URLs:           append([]string{}, queueOpts.NATS.URLs...),
				SubjectPrefix:  strings.TrimSpace(queueOpts.NATS.SubjectPrefix),
				QueueGroup:     strings.TrimSpace(queueOpts.NATS.QueueGroup),
				PollTimeout:    time.Duration(queueOpts.NATS.PollTimeoutMs) * time.Millisecond,
				FallbackDriver: redisTaskDriver,
			})
			pxlog.WarnF(context.Background(), "[event_fabric.task_driver] queue.driver=nats enabled; using nats adapter with redis fallback")
			retryWorkerDriverName = string(taskDriver.Type())
		}
		scheduler = deliveryService.NewBackoffScheduler(reliableQueue)
		if metricsRecorder != nil {
			capability := taskDriver.Capability()
			metricsRecorder.ObserveTaskDriverInit(context.Background(), string(taskDriver.Type()), capability.SupportsBlockingDequeue)
			pxlog.InfoF(context.Background(), "[event_fabric.task_driver] initialized driver=%s blocking=%s ack_timeout=%s",
				taskDriver.Type(), cfg.SchedulerInterval, cfg.AckTimeout)
		}
	}
	if taskDriver != nil && db != nil {
		taskDriver = newTaskHistoryDriver(
			taskDriver,
			eventfabricrepo.NewTaskHistoryRepository(db),
			time.Now,
		)
	}

	topicLookup := eventshared.NewCachedTopicLookup(eventfabricrepo.NewTopicRepository(db), eventshared.CachedTopicLookupOptions{
		Cache: func() cache.ICache {
			if redisClient == nil {
				return nil
			}
			return cache.NewRedisCache(redisClient)
		}(),
		TTL:     180 * time.Second,
		MissTTL: 30 * time.Second,
	})

	aclSvc := aclService.NewACLService(aclService.Options{
		DB:         db,
		TopicStore: topicLookup,
		Clock:      time.Now,
	})
	directorySvc := directoryService.NewDirectoryService(directoryService.Options{
		DB:                db,
		ACL:               aclSvc,
		EventBus:          bus,
		Clock:             time.Now,
		ActorResolver:     func(context.Context) string { return "system" },
		DefaultMaxRetry:   cfg.DefaultMaxRetry,
		DefaultAckTimeout: cfg.AckTimeout,
	})
	aclEnforcer := aclService.NewACLEnforcer(aclSvc)
	bindingRepo := eventfabricrepo.NewManifestBindingRepository(db)

	auditSvcEF := auditService.NewService(auditService.Options{
		AuditService: auditSvc,
		Source:       "event_fabric",
		ResourceType: "event",
		Clock:        time.Now,
	})

	if securityVerifier != nil {
		securityVerifier.SetViolationReporter(auditViolationReporter{audit: auditSvcEF})
	}

	var seedSvc *manifestService.SeedService
	var bindingStore manifestService.BindingStore
	if bindingRepo != nil {
		bindingStore = manifestService.NewBindingStore(bindingRepo)
	}

	if directorySvc != nil && aclSvc != nil {
		seedSvc = manifestService.NewSeedService(manifestService.SeedServiceOptions{
			Directory: directorySvc,
			ACL:       aclSvc,
			Audit:     auditSvcEF,
			Logger:    pxlog.GetGlobalLogger(),
			Clock:     time.Now,
			Bindings:  bindingStore,
		})
	}

	var deliverySvc deliveryService.Service
	if scheduler != nil {
		var err error
		deliverySvc, err = deliveryService.NewService(deliveryService.Options{
			DB:                           db,
			Topics:                       topicLookup,
			ACL:                          aclSvc,
			Scheduler:                    scheduler,
			Clock:                        time.Now,
			MaxRetry:                     cfg.DefaultMaxRetry,
			Audit:                        auditSvcEF,
			Metrics:                      metricsRecorder,
			EnableDatabaseFallbackLookup: retryWorkerFallbackEnabled,
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
			Topics:   topicLookup,
			ACL:      aclSvc,
			Delivery: deliverySvc,
			History:  eventfabricrepo.NewTaskHistoryRepository(db),
			Clock:    time.Now,
			Metrics:  metricsRecorder,
			Emitter:  newReplayTaskWSStatusEmitter(),
		})
	}

	var retryWorker *workers.EventFabricRetryWorker
	if deliverySvc != nil && bus != nil {
		tenantProvider := newRetryQueueTenantKeyProvider(redisClient, 10*time.Second)
		if tenantProvider == nil {
			tenantProvider = newCachedTenantKeyProvider(tenantSvc, 30*time.Second)
		}
		retryWorker = workers.NewEventFabricRetryWorker(workers.EventFabricRetryWorkerOptions{
			Delivery:                deliverySvc,
			EventBus:                bus,
			TenantProvider:          tenantProvider,
			Interval:                cfg.SchedulerInterval,
			BatchSize:               100,
			EnableDBPollingFallback: retryWorkerFallbackEnabled,
			DriverName:              retryWorkerDriverName,
		})
	}

	var cronDispatcherWorker *workers.EventFabricCronDispatcherWorker
	if db != nil && taskDriver != nil {
		taskRepo := eventfabricrepo.NewScheduledTaskRepository(db)
		taskRunRepo := eventfabricrepo.NewScheduledTaskRunRepository(db)
		cronDispatcherWorker = workers.NewEventFabricCronDispatcherWorker(workers.EventFabricCronDispatcherWorkerOptions{
			TaskRepository:    taskRepo,
			TaskRunRepository: taskRunRepo,
			TaskDriver:        taskDriver,
			Scheduler:         cronschedulersvc.NewService(),
			SubscriberID:      eventbus.SubscriberEventFabricCronDispatch,
			Topic:             "event_fabric.cron.dispatch",
			Interval:          cfg.SchedulerInterval,
			BatchSize:         100,
			Logger:            pxlog.GetGlobalLogger(),
			Clock:             time.Now,
		})
	}
	var notificationWorker *workers.EventFabricSystemNotificationDispatchWorker
	if taskDriver != nil {
		notificationWorker = workers.NewEventFabricSystemNotificationDispatchWorker(workers.EventFabricSystemNotificationDispatchWorkerOptions{
			TaskDriver:   taskDriver,
			SubscriberID: eventbus.SubscriberSystemNotificationDispatch,
			TenantKey:    "global",
			BatchSize:    100,
			WaitTimeout:  3 * time.Second,
			RetryDelay:   5 * time.Second,
			Publish: func(tenantKey, topic string, payload any, traceID string) {
				wsbus.DefaultHub.Publish(tenantKey, topic, payload, traceID)
			},
			Logger: pxlog.GetGlobalLogger(),
			Clock:  time.Now,
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

		var timeoutTaskWorker *workers.EventFabricAuthorizationTimeoutTaskWorker
		if authService != nil && taskDriver != nil && bus != nil {
			workers.RegisterAuthorizationChallengeTimeoutTaskScheduler(
				bus,
				taskDriver,
				authCfg.ChallengeTopic,
				pxlog.GetGlobalLogger(),
				time.Now,
			)
			timeoutTaskWorker = workers.NewEventFabricAuthorizationTimeoutTaskWorker(workers.EventFabricAuthorizationTimeoutTaskWorkerOptions{
				Service:      authService,
				TaskDriver:   taskDriver,
				SubscriberID: eventbus.SubscriberAuthorizationChallengeTime,
				TenantKey:    "global",
				BatchSize:    100,
				WaitTimeout:  3 * time.Second,
				RetryDelay:   5 * time.Second,
				Logger:       pxlog.GetGlobalLogger(),
				Clock:        time.Now,
			})
		}

		authDeps = &AuthorizationDeps{
			Service:           authService,
			Templates:         templateService,
			Cache:             cache,
			Dispatcher:        dispatcher,
			Secrets:           secretsManager,
			Limiter:           rateLimiter,
			Alerts:            alertEmitter,
			Reporting:         reportingService,
			TimeoutTaskWorker: timeoutTaskWorker,
		}
	}

	return &EventFabricDeps{
		RedisClient:          redisClient,
		EventBus:             bus,
		Config:               cfg,
		Directory:            directorySvc,
		ACL:                  aclSvc,
		Enforcer:             aclEnforcer,
		Seeder:               seedSvc,
		BindingStore:         bindingStore,
		Reliable:             reliableQueue,
		TaskDriver:           taskDriver,
		Scheduler:            scheduler,
		Delivery:             deliverySvc,
		DLQ:                  dlqSvc,
		Audit:                auditSvcEF,
		Replay:               replaySvc,
		RetryWorker:          retryWorker,
		CronDispatcherWorker: cronDispatcherWorker,
		NotificationWorker:   notificationWorker,
		Metrics:              metricsRecorder,
		Security:             securityVerifier,
		Authorization:        authDeps,
	}
}

func newAgentLifecycleDeps(db *gorm.DB, opts AgentLifecycleOptions, bus event_bus.EventBus, auditSvc auditsvc.Service) *AgentLifecycleDeps {
	var redisClient *redis.Client
	if addr := strings.TrimSpace(opts.RedisAddr); addr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: opts.RedisPassword,
			DB:       opts.RedisDB,
		})
	}

	alertCooldown := opts.Notifications.RetryInterval
	inst := agentinstr.New(agentinstr.Options{
		Audit:         auditSvc,
		AlertCooldown: alertCooldown,
	})

	notifier := imnotify.NewSender(imnotify.Config{
		WebhookURL:    opts.Notifications.IMWebhook,
		RetryInterval: opts.Notifications.RetryInterval,
		MaxRetry:      opts.Notifications.RetryMaxAttempts,
		HTTPTimeout:   opts.Notifications.HTTPTimeout,
	})

	profileRepo := agentrepo.NewAgentProfileLifecycleRepository(db)
	lifecycleRepo := agentrepo.NewAgentLifecycleEventRepository(db)
	healthRepo := agentrepo.NewAgentHealthSnapshotRepository(db)
	shareRepo := agentrepo.NewAgentShareRepository(db)
	tenantFormRepo := agentrepo.NewAgentTenantFormRepository(db)
	policyEngine := agentlifecycle.NewDefaultPolicyConflictEngine(agentlifecycle.PolicyEngineOptions{})
	approvalFlow := workflow.NewAgentApprovalFlow()
	shareValidator := agentlifecycle.NewTenantShareValidator(policyEngine, nil)
	quotaProvisioner := agentlifecycle.NewDefaultQuotaProvisioner()

	service := agentlifecycle.NewService(agentlifecycle.ServiceOptions{
		ProfileRepo:     profileRepo,
		LifecycleRepo:   lifecycleRepo,
		HealthRepo:      healthRepo,
		ShareRepo:       shareRepo,
		TenantFormRepo:  tenantFormRepo,
		EventBus:        bus,
		Instrumentation: inst,
		Notifier:        notifier,
		Config: agentlifecycle.Config{
			DefaultCapacityInstances: opts.DefaultCapacityInstances,
			EventTopics: agentlifecycle.EventTopics{
				LifecyclePrefix: opts.EventTopics.LifecyclePrefix,
				HealthPrefix:    opts.EventTopics.HealthPrefix,
			},
			AlertCooldown: alertCooldown,
			StateBusTopics: agentlifecycle.StateBusTopics{
				Lifecycle: opts.StateBusTopics.Lifecycle,
				Health:    opts.StateBusTopics.Health,
			},
			ShareReviewInterval: opts.ShareReviewInterval,
		},
		Clock:            time.Now,
		PolicyEngine:     policyEngine,
		ApprovalFlow:     approvalFlow,
		ShareValidator:   shareValidator,
		QuotaProvisioner: quotaProvisioner,
	})

	return &AgentLifecycleDeps{
		ProfileRepo:     profileRepo,
		LifecycleRepo:   lifecycleRepo,
		HealthRepo:      healthRepo,
		ShareRepo:       shareRepo,
		TenantFormRepo:  tenantFormRepo,
		Instrumentation: inst,
		Notifications:   notifier,
		RedisClient:     redisClient,
		EventBus:        bus,
		Config: AgentLifecycleRuntimeConfig{
			CapacityKeyPrefix:        opts.CapacityKeyPrefix,
			HealthKeyPrefix:          opts.HealthKeyPrefix,
			DefaultCapacityInstances: opts.DefaultCapacityInstances,
			EventTopics: AgentLifecycleEventTopicsOptions{
				LifecyclePrefix: opts.EventTopics.LifecyclePrefix,
				HealthPrefix:    opts.EventTopics.HealthPrefix,
			},
		},
		Service:          service,
		PolicyEngine:     policyEngine,
		ApprovalFlow:     approvalFlow,
		ShareValidator:   shareValidator,
		QuotaProvisioner: quotaProvisioner,
	}
}

func newKnowledgeSpaceDeps(db *gorm.DB, opts KnowledgeSpaceOptions, bus event_bus.EventBus, auditSvc auditsvc.Service, eventFabric *EventFabricDeps) *KnowledgeSpaceDeps {
	var redisClient *redis.Client
	if addr := strings.TrimSpace(opts.RedisAddr); addr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: opts.RedisPassword,
			DB:       opts.RedisDB,
		})
	}

	inst := knowledgeinstr.New(knowledgeinstr.Options{
		Audit: auditSvc,
	})

	var vectorStore vectorstorepkg.Store
	driverName := strings.ToLower(strings.TrimSpace(opts.VectorStore.Driver))
	if driverName != "" {
		var driverCfg interface{}
		switch driverName {
		case vectorstorepkg.DriverPGVector:
			driverCfg = opts.VectorStore.PGVector.WithDefaults()
		case vectorstorepkg.DriverMilvus:
			driverCfg = opts.VectorStore.Milvus
		case vectorstorepkg.DriverPinecone:
			driverCfg = opts.VectorStore.Pinecone
		default:
			pxlog.WarnF(context.Background(), "[knowledge_space] 不支持的向量存储驱动: %s", driverName)
		}
		if driverCfg != nil {
			store, err := vectorstorepkg.Open(driverName, driverCfg)
			if err != nil {
				if err == vectorstorepkg.ErrNotImplemented {
					pxlog.WarnF(context.Background(), "[knowledge_space] 向量存储驱动 %s 暂未实现", driverName)
				} else {
					pxlog.WarnF(context.Background(), "[knowledge_space] 初始化向量存储失败: %v", err)
				}
			} else {
				vectorStore = store
			}
		}
	}

	cfg := KnowledgeSpaceRuntimeConfig{
		LockKeyPrefix:            strings.TrimSpace(opts.LockKeyPrefix),
		MetricsKeyPrefix:         strings.TrimSpace(opts.MetricsKeyPrefix),
		DefaultRetentionMonths:   opts.DefaultRetentionMonths,
		ProvisioningSLA:          opts.ProvisioningSLA,
		IngestionSLA:             opts.IngestionSLA,
		SceneStrategyCatalogPath: strings.TrimSpace(opts.SceneStrategyCatalogPath),
		EventTopics:              opts.EventTopics,
		Notifications:            opts.Notifications,
	}

	if cfg.LockKeyPrefix == "" {
		cfg.LockKeyPrefix = "knowledge_space:lock"
	}
	if cfg.MetricsKeyPrefix == "" {
		cfg.MetricsKeyPrefix = "knowledge_space:metrics"
	}
	if cfg.DefaultRetentionMonths <= 0 {
		cfg.DefaultRetentionMonths = 13
	}
	if cfg.ProvisioningSLA <= 0 {
		cfg.ProvisioningSLA = 2 * time.Minute
	}
	if cfg.IngestionSLA <= 0 {
		cfg.IngestionSLA = 4 * time.Hour
	}
	if cfg.SceneStrategyCatalogPath == "" {
		cfg.SceneStrategyCatalogPath = "backend/config/knowledge/scene_strategy_catalog.yaml"
	}
	if cfg.EventTopics.Provisioning == "" {
		cfg.EventTopics.Provisioning = "knowledge.space.provisioning"
	}
	if cfg.EventTopics.Ingestion == "" {
		cfg.EventTopics.Ingestion = "knowledge.space.ingestion"
	}
	if cfg.EventTopics.Fusion == "" {
		cfg.EventTopics.Fusion = "knowledge.space.fusion"
	}
	if cfg.EventTopics.Feedback == "" {
		cfg.EventTopics.Feedback = "knowledge.space.feedback"
	}
	if cfg.Notifications.RetryInterval <= 0 {
		cfg.Notifications.RetryInterval = time.Minute
	}
	if cfg.Notifications.HTTPTimeout <= 0 {
		cfg.Notifications.HTTPTimeout = 5 * time.Second
	}
	if cfg.Notifications.RetryMaxAttempts <= 0 {
		cfg.Notifications.RetryMaxAttempts = 3
	}

	serviceCfg := knowledgeService.RuntimeConfig{
		LockKeyPrefix:            cfg.LockKeyPrefix,
		DefaultRetentionMonths:   cfg.DefaultRetentionMonths,
		ProvisioningSLA:          cfg.ProvisioningSLA,
		SceneStrategyCatalogPath: strings.TrimSpace(cfg.SceneStrategyCatalogPath),
		EventTopics: knowledgeService.EventTopics{
			Provisioning: cfg.EventTopics.Provisioning,
			Ingestion:    cfg.EventTopics.Ingestion,
			Fusion:       cfg.EventTopics.Fusion,
			Feedback:     cfg.EventTopics.Feedback,
		},
	}

	svc := knowledgeService.NewService(knowledgeService.ServiceOptions{
		DB:              db,
		Instrumentation: inst,
		Redis:           redisClient,
		EventBus:        bus,
		Config:          serviceCfg,
		Clock:           time.Now,
	})

	metricsWriter := knowledgeService.NewIngestionMetricsWriter("")
	feedbackReportPath := strings.TrimSpace(opts.Reports.FeedbackPath)
	if feedbackReportPath == "" {
		feedbackReportPath = filepath.Join("backend", "reports", "_state", "knowledge-feedback.json")
	}
	aggregateReportPath := strings.TrimSpace(opts.Delta.AggregateReportPath)
	if aggregateReportPath == "" {
		aggregateReportPath = filepath.Join("reports", "_state", "knowledge-update.json")
	}
	deltaReportPath := strings.TrimSpace(opts.Delta.ReportPath)
	if deltaReportPath == "" {
		deltaReportPath = filepath.Join("backend", "reports", "_state", "knowledge-delta.json")
	}
	qaBridgeReportPath := strings.TrimSpace(opts.Reports.QABridgePath)
	if qaBridgeReportPath == "" {
		qaBridgeReportPath = filepath.Join("reports", "_state", "qa-reasoning.json")
	}
	eventReportPath := strings.TrimSpace(opts.EventHotfix.ReportPath)
	if eventReportPath == "" {
		eventReportPath = filepath.Join("backend", "reports", "_state", "knowledge-event.json")
	}
	releaseReportPath := strings.TrimSpace(opts.Release.ReportPath)
	if releaseReportPath == "" {
		releaseReportPath = filepath.Join("backend", "reports", "_state", "knowledge-release.json")
	}
	decayReportPath := strings.TrimSpace(opts.Decay.ReportPath)
	if decayReportPath == "" {
		decayReportPath = filepath.Join("backend", "reports", "_state", "knowledge-decay.json")
	}
	eventAggregatePath := strings.TrimSpace(opts.EventHotfix.AggregateReportPath)
	if eventAggregatePath == "" {
		eventAggregatePath = aggregateReportPath
	}
	releaseAggregatePath := strings.TrimSpace(opts.Release.AggregateReportPath)
	if releaseAggregatePath == "" {
		releaseAggregatePath = aggregateReportPath
	}
	decayAggregatePath := strings.TrimSpace(opts.Decay.AggregateReportPath)
	if decayAggregatePath == "" {
		decayAggregatePath = aggregateReportPath
	}
	feedbackMetricsWriter := knowledgeService.NewFeedbackMetricsWriter(feedbackReportPath, aggregateReportPath)
	deltaMetricsWriter := knowledgeinstr.NewDeltaMetricsWriter(deltaReportPath, aggregateReportPath)
	eventMetricsWriter := knowledgeinstr.NewEventMetricsWriter(eventReportPath, eventAggregatePath)
	releaseMetricsWriter := knowledgeinstr.NewReleaseMetricsWriter(releaseReportPath, releaseAggregatePath)
	decayMetricsWriter := knowledgeinstr.NewDecayMetricsWriter(decayReportPath, decayAggregatePath)

	processors := knowledgeService.NewProcessorRegistry()
	// config.yaml 显式配置优先于自动探测/环境变量
	if opts.IngestionProcessors.PDFTextAvailable != nil {
		processors.SetPDFTextAvailable(*opts.IngestionProcessors.PDFTextAvailable)
	}
	if opts.IngestionProcessors.OCRAvailable != nil {
		processors.SetOCRAvailable(*opts.IngestionProcessors.OCRAvailable)
	}

	agentSettingSvc := agentsettings.NewAgentSettingService(db)

	routedVectorStore := vectorStore
	if strings.EqualFold(strings.TrimSpace(driverName), vectorstorepkg.DriverPGVector) {
		routedVectorStore = knowledgeService.NewRoutedVectorStore(knowledgeService.RoutedVectorStoreOptions{
			DB:         db,
			BaseDriver: driverName,
			BaseStore:  vectorStore,
			PGVector:   opts.VectorStore.PGVector,
		})
	}
	ingestionSvc := knowledgeService.NewIngestionService(knowledgeService.IngestionServiceOptions{
		DB:              db,
		Instrumentation: inst,
		VectorStore:     routedVectorStore,
		MetricsWriter:   metricsWriter,
		Processors:      processors,
		MaxRetries:      1,
		AgentSettings:   agentSettingSvc,
		VectorDimension: 0,
		ProgressPublisher: knowledgeService.IngestionProgressPublisherFunc(func(ctx context.Context, update knowledgeService.IngestionProgressUpdate) {
			if strings.TrimSpace(update.TenantUUID) == "" {
				return
			}
			wsbus.DefaultHub.Publish(update.TenantUUID, eventbus.TopicKnowledgeIngestionJob, update, reqctx.GetTraceID(ctx))
		}),
	})
	svc.AttachIngestion(ingestionSvc)

	fusionSvc := knowledgeService.NewFusionService(knowledgeService.FusionServiceOptions{
		DB:              db,
		Instrumentation: inst,
		VectorStore:     routedVectorStore,
		SparseIndex:     nil,
		EventBus:        bus,
		EventTopic:      cfg.EventTopics.Fusion,
		Clock:           time.Now,
	})

	reprocessTopic := cfg.EventTopics.Feedback + ".reprocess"
	var reprocessPipeline knowledgeworkflow.ReprocessPipeline
	if eventFabric != nil && eventFabric.Delivery != nil && eventFabric.Directory != nil && eventFabric.ACL != nil {
		reprocessPipeline = knowledgeworkflow.NewEventFabricReprocessPipeline(knowledgeworkflow.EventFabricReprocessPipelineOptions{
			Delivery:      eventFabric.Delivery,
			Directory:     eventFabric.Directory,
			ACL:           eventFabric.ACL,
			SubscriberID:  eventbus.SubscriberKnowledgeSpaceReprocess,
			Namespace:     cfg.EventTopics.Feedback,
			Name:          "reprocess",
			PayloadFormat: "json",
			MaxRetry:      int32(eventFabric.Config.DefaultMaxRetry),
			AckTimeoutSec: int32(eventFabric.Config.AckTimeout / time.Second),
			Clock:         time.Now,
		})
		knowledgeworkflow.RegisterEventFabricReprocessDispatchHandler(knowledgeworkflow.EventFabricReprocessDispatchHandlerOptions{
			EventBus:     bus,
			DB:           db,
			VectorStore:  routedVectorStore,
			SubscriberID: eventbus.SubscriberKnowledgeSpaceReprocess,
			Clock:        time.Now,
		})
	} else {
		reprocessPipeline = knowledgeworkflow.NewReprocessPipeline(knowledgeworkflow.ReprocessPipelineOptions{
			EventBus:   bus,
			EventTopic: reprocessTopic,
			Clock:      time.Now,
		})
		_ = knowledgeworkflow.NewReprocessWorker(knowledgeworkflow.ReprocessWorkerOptions{
			DB:          db,
			VectorStore: routedVectorStore,
			EventBus:    bus,
			EventTopic:  reprocessTopic,
			Clock:       time.Now,
		}).Start()
	}
	feedbackSvc := knowledgeService.NewFeedbackService(knowledgeService.FeedbackServiceOptions{
		DB:              db,
		Instrumentation: inst,
		Pipeline:        reprocessPipeline,
		MetricsWriter:   metricsWriter,
		FeedbackMetrics: feedbackMetricsWriter,
		Clock:           time.Now,
	})

	var corpusCheckPipeline knowledgeworkflow.CorpusCheckPipeline
	if eventFabric != nil && eventFabric.Delivery != nil && eventFabric.Directory != nil && eventFabric.ACL != nil {
		corpusCheckPipeline = knowledgeworkflow.NewEventFabricCorpusCheckPipeline(knowledgeworkflow.EventFabricCorpusCheckPipelineOptions{
			Delivery:      eventFabric.Delivery,
			Directory:     eventFabric.Directory,
			ACL:           eventFabric.ACL,
			SubscriberID:  eventbus.SubscriberKnowledgeSpaceCorpusCheck,
			Namespace:     "_topic.knowledge.space.corpuscheck",
			Name:          "run",
			PayloadFormat: "json",
			MaxRetry:      int32(eventFabric.Config.DefaultMaxRetry),
			AckTimeoutSec: int32(eventFabric.Config.AckTimeout / time.Second),
			Clock:         time.Now,
		})
		knowledgeworkflow.RegisterEventFabricCorpusCheckDispatchHandler(knowledgeworkflow.EventFabricCorpusCheckDispatchHandlerOptions{
			EventBus:     bus,
			DB:           db,
			SubscriberID: eventbus.SubscriberKnowledgeSpaceCorpusCheck,
			Clock:        time.Now,
		})
	} else {
		corpusCheckPipeline = knowledgeworkflow.NewCorpusCheckPipeline(knowledgeworkflow.CorpusCheckPipelineOptions{
			EventBus:   bus,
			EventTopic: "knowledge.corpus_check.run",
			Clock:      time.Now,
		})
		_ = knowledgeworkflow.NewCorpusCheckWorker(knowledgeworkflow.CorpusCheckWorkerOptions{
			DB:         db,
			EventBus:   bus,
			EventTopic: "knowledge.corpus_check.run",
			Clock:      time.Now,
		}).Start()
	}
	corpusCheckSvc := knowledgeService.NewCorpusCheckService(knowledgeService.CorpusCheckServiceOptions{
		DB:       db,
		Pipeline: corpusCheckPipeline,
		Clock:    time.Now,
	})

	deltaSvc := ksdelta.NewService(ksdelta.Options{
		DB:                       db,
		Instrumentation:          inst,
		MetricsWriter:            deltaMetricsWriter,
		SourcesConfigPath:        opts.Delta.SourcesConfig,
		PartialReleaseConfigPath: opts.Delta.PartialReleaseConfig,
		Clock:                    time.Now,
	})

	snapshotStore := knctxsnapshot.NewStore()
	toolRegistry := kntoolchain.NewRegistry()
	guard := kncompliance.NewGuard()
	qaBridgeSvc := knowledgeqa.NewService(knowledgeqa.Options{
		DB:              db,
		Instrumentation: inst,
		VectorStore:     routedVectorStore,
		SnapshotStore:   snapshotStore,
		ToolRegistry:    toolRegistry,
		Guard:           guard,
		Clock:           time.Now,
		ReportPath:      qaBridgeReportPath,
	})

	agentNotifier := event_hotfix.NewAgentNotifier(opts.EventHotfix.AgentMatrixPath)
	eventHotfixSvc := event_hotfix.NewService(event_hotfix.Options{
		DB:              db,
		Instrumentation: inst,
		EventBus:        bus,
		MetricsWriter:   eventMetricsWriter,
		AgentNotifier:   agentNotifier,
		PoliciesPath:    opts.EventHotfix.PoliciesPath,
		ReportPath:      eventReportPath,
		Clock:           time.Now,
		RetryMax:        opts.EventHotfix.RetryMax,
		ReplayWindow:    opts.EventHotfix.ReplayWindow,
	})
	releaseSvc := tenant_release.NewService(tenant_release.Options{
		DB:              db,
		Instrumentation: inst,
		MetricsWriter:   releaseMetricsWriter,
		Clock:           time.Now,
	})
	decaySvc := decay_guard.NewService(decay_guard.Options{
		DB:              db,
		Instrumentation: inst,
		MetricsWriter:   decayMetricsWriter,
		ThresholdsPath:  opts.Decay.ThresholdPath,
		EventBus:        bus,
		Clock:           time.Now,
	})

	return &KnowledgeSpaceDeps{
		Instrumentation: inst,
		RedisClient:     redisClient,
		EventBus:        bus,
		Config:          cfg,
		Service:         svc,
		Ingestion:       ingestionSvc,
		Fusion:          fusionSvc,
		Feedback:        feedbackSvc,
		CorpusCheck:     corpusCheckSvc,
		Delta:           deltaSvc,
		EventHotfix:     eventHotfixSvc,
		DecayGuard:      decaySvc,
		Release:         releaseSvc,
		VectorStore:     routedVectorStore,
		QABridge:        qaBridgeSvc,
	}
}

func newIntegrationGatewayDeps(db *gorm.DB, opts IntegrationGatewayOptions, bus event_bus.EventBus, auditor auditsvc.Auditor) *IntegrationGatewayDeps {
	prefix := strings.TrimSpace(opts.RateLimitPrefix)
	if prefix == "" {
		prefix = "integration_gateway:rl"
	}

	var redisClient *redis.Client
	if addr := strings.TrimSpace(opts.RedisAddr); addr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: opts.RedisPassword,
			DB:       opts.RedisDB,
		})
	}

	window := time.Duration(opts.DefaultRateLimit.WindowSeconds) * time.Second
	if window <= 0 {
		window = time.Minute
	}

	policy := authorizationService.RateLimitPolicy{
		Limit:    opts.DefaultRateLimit.Limit,
		Burst:    opts.DefaultRateLimit.Burst,
		Interval: window,
	}

	limiterOpts := authorizationService.RateLimiterOptions{
		Client: redisClient,
		Prefix: prefix,
	}
	limiter := authorizationService.NewRateLimiter(limiterOpts)

	inst := integrationInstrumentation.NewInstrumentation(nil)

	managerConfig := integrationManager.Config{
		RateLimitPrefix: prefix,
		DefaultRateLimit: integrationManager.RateLimitPolicy{
			Limit:         opts.DefaultRateLimit.Limit,
			Burst:         opts.DefaultRateLimit.Burst,
			WindowSeconds: opts.DefaultRateLimit.WindowSeconds,
			Scope:         opts.DefaultRateLimit.Scope,
		},
		EventTopics: integrationManager.EventTopics(opts.EventTopics),
	}

	routeRepo := integrationRepo.NewIntegrationRouteRepository(db)
	versionRepo := integrationRepo.NewIntegrationRouteVersionRepository(db)
	eventRepo := integrationRepo.NewIntegrationEventPublicationRepository(db)

	managerService := integrationManager.NewService(integrationManager.ServiceOptions{
		DB:              db,
		RouteRepo:       routeRepo,
		VersionRepo:     versionRepo,
		EventRepo:       eventRepo,
		EventBus:        bus,
		Instrumentation: inst,
		Auditor:         auditor,
		Config:          managerConfig,
		Clock:           time.Now,
	})

	return &IntegrationGatewayDeps{
		RateLimiter: limiter,
		RedisClient: redisClient,
		Config: IntegrationGatewayRuntimeConfig{
			RateLimitPrefix: prefix,
			DefaultPolicy:   policy,
			EventTopics:     opts.EventTopics,
		},
		Manager:         managerService,
		Instrumentation: inst,
	}
}

func newRetryQueueTenantKeyProvider(redisClient *redis.Client, ttl time.Duration) func(context.Context) ([]string, error) {
	if redisClient == nil {
		return nil
	}
	if ttl <= 0 {
		ttl = 10 * time.Second
	}
	const retryKeyPrefix = "event:retry:"
	var mu sync.Mutex
	var cached []string
	var expireAt time.Time

	return func(ctx context.Context) ([]string, error) {
		now := time.Now()
		forceRefresh := workers.RetryTenantProviderBypassCache(ctx)
		mu.Lock()
		if !forceRefresh && now.Before(expireAt) && len(cached) > 0 {
			out := append([]string(nil), cached...)
			mu.Unlock()
			return out, nil
		}
		mu.Unlock()

		tenants := make(map[string]struct{}, 8)
		cursor := uint64(0)
		for {
			keys, nextCursor, err := redisClient.Scan(ctx, cursor, retryKeyPrefix+"*", 200).Result()
			if err != nil {
				return nil, err
			}
			for _, key := range keys {
				tenant := strings.TrimSpace(strings.TrimPrefix(key, retryKeyPrefix))
				if tenant != "" {
					tenants[tenant] = struct{}{}
				}
			}
			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}

		filtered := make([]string, 0, len(tenants)+1)
		for tenant := range tenants {
			filtered = append(filtered, tenant)
		}
		if len(filtered) == 0 {
			filtered = append(filtered, "global")
		}

		mu.Lock()
		cached = append([]string(nil), filtered...)
		expireAt = time.Now().Add(ttl)
		mu.Unlock()

		return filtered, nil
	}
}

func newCachedTenantKeyProvider(tenantSvc *tenantsvc.TenantService, ttl time.Duration) func(context.Context) ([]string, error) {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	var mu sync.Mutex
	var cached []string
	var expireAt time.Time

	return func(ctx context.Context) ([]string, error) {
		if tenantSvc == nil {
			return []string{"global"}, nil
		}

		now := time.Now()
		forceRefresh := workers.RetryTenantProviderBypassCache(ctx)
		mu.Lock()
		if !forceRefresh && now.Before(expireAt) && len(cached) > 0 {
			out := append([]string(nil), cached...)
			mu.Unlock()
			return out, nil
		}
		mu.Unlock()

		keys, err := tenantSvc.Repo.ListActiveKeys(ctx)
		if err != nil {
			return nil, err
		}
		filtered := make([]string, 0, len(keys)+1)
		for _, key := range keys {
			if key = strings.TrimSpace(key); key != "" {
				filtered = append(filtered, key)
			}
		}
		filtered = append(filtered, "global")

		mu.Lock()
		cached = append([]string(nil), filtered...)
		expireAt = time.Now().Add(ttl)
		mu.Unlock()

		return filtered, nil
	}
}
