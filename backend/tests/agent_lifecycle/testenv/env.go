package testenv

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	imnotify "github.com/ArtisanCloud/PowerX/internal/notifications/im"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentrepo "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	"github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle"
	agentinstr "github.com/ArtisanCloud/PowerX/internal/service/agent_lifecycle/instrumentation"
	agentgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/agentlifecycle"
	adminhttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agentlifecycle"
	agentopenapi "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/agent"
	"github.com/ArtisanCloud/PowerX/internal/workflow"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Env 为 Agent Lifecycle 测试提供独立环境。
type Env struct {
	T                 testing.TB
	DB                *gorm.DB
	Deps              *shared.Deps
	Bus               event_bus.EventBus
	Server            *agentgrpc.Server
	Notifier          *MockNotifier
	ManifestValidator *MockManifestValidator
	SandboxRunner     *MockSandboxRunner
	ShareValidator    *MockShareValidator
	QuotaProvisioner  *MockQuotaProvisioner
}

// New 构造测试环境。
func New(t testing.TB) *Env {
	t.Helper()

	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	coremodel.PowerXSchema = "main"
	if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	if err := db.AutoMigrate(
		&agentmodel.AgentProfileLifecycle{},
		&agentmodel.AgentLifecycleEventRecord{},
		&agentmodel.AgentHealthSnapshotRecord{},
		&agentmodel.AgentShareRecord{},
		&agentmodel.AgentTenantForm{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_health_window ON agent_health_snapshots(agent_uuid, window_started_at)").Error; err != nil {
		t.Fatalf("create unique index: %v", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_profile_tenant_alias_unique ON agent_profiles(tenant_uuid, alias)").Error; err != nil {
		t.Fatalf("create alias unique index: %v", err)
	}

	bus := event_bus.NewLocalEventBus()
	inst := agentinstr.New(agentinstr.Options{})

	notifier := NewMockNotifier()

	profileRepo := agentrepo.NewAgentProfileLifecycleRepository(db)
	eventRepo := agentrepo.NewAgentLifecycleEventRepository(db)
	healthRepo := agentrepo.NewAgentHealthSnapshotRepository(db)
	shareRepo := agentrepo.NewAgentShareRepository(db)
	tenantFormRepo := agentrepo.NewAgentTenantFormRepository(db)
	policyEngine := agent_lifecycle.NewDefaultPolicyConflictEngine(agent_lifecycle.PolicyEngineOptions{})
	approvalFlow := workflow.NewAgentApprovalFlow()

	manifestValidator := NewMockManifestValidator()
	sandboxRunner := NewMockSandboxRunner()
	shareValidator := NewMockShareValidator()
	quotaProvisioner := NewMockQuotaProvisioner()

	service := agent_lifecycle.NewService(agent_lifecycle.ServiceOptions{
		ProfileRepo:     profileRepo,
		LifecycleRepo:   eventRepo,
		HealthRepo:      healthRepo,
		ShareRepo:       shareRepo,
		TenantFormRepo:  tenantFormRepo,
		EventBus:        bus,
		Notifier:        notifier,
		Instrumentation: inst,
		Config: agent_lifecycle.Config{
			DefaultCapacityInstances: 3,
			EventTopics: agent_lifecycle.EventTopics{
				LifecyclePrefix: "agent.lifecycle",
				HealthPrefix:    "agent.health",
			},
			StateBusTopics: agent_lifecycle.StateBusTopics{
				Lifecycle: "statebus.agent.lifecycle",
				Health:    "statebus.agent.health",
			},
			ShareReviewInterval: 24 * time.Hour,
		},
		Clock:             time.Now,
		PolicyEngine:      policyEngine,
		ApprovalFlow:      approvalFlow,
		ManifestValidator: manifestValidator,
		SandboxRunner:     sandboxRunner,
		ShareValidator:    shareValidator,
		QuotaProvisioner:  quotaProvisioner,
	})

	agentDeps := &shared.AgentLifecycleDeps{
		ProfileRepo:     profileRepo,
		LifecycleRepo:   eventRepo,
		HealthRepo:      healthRepo,
		ShareRepo:       shareRepo,
		TenantFormRepo:  tenantFormRepo,
		Instrumentation: inst,
		Notifications:   notifier,
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
		Service:          service,
		PolicyEngine:     policyEngine,
		ApprovalFlow:     approvalFlow,
		ShareValidator:   shareValidator,
		QuotaProvisioner: quotaProvisioner,
	}

	deps := &shared.Deps{
		EventBus:       bus,
		AgentLifecycle: agentDeps,
	}

	return &Env{
		T:                 t,
		DB:                db,
		Deps:              deps,
		Bus:               bus,
		Server:            agentgrpc.NewServer(service),
		Notifier:          notifier,
		ManifestValidator: manifestValidator,
		SandboxRunner:     sandboxRunner,
		ShareValidator:    shareValidator,
		QuotaProvisioner:  quotaProvisioner,
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
		tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
		if tenantUUID == "" {
			tenantUUID = "tenant-auto"
		}
		ctx := reqctx.WithTenantUUID(c.Request.Context(), tenantUUID)
		c.Request = c.Request.WithContext(ctx)
		reqctx.CopyCtxToGin(c)
		c.Next()
	})
	adminhttp.Register(public, protected, e.Deps)
	agentopenapi.Register(public, protected, e.Deps)
	return engine
}

// GRPCServer 返回实现 AgentLifecycleService 的 gRPC Server。
func (e *Env) GRPCServer() grpc.ServiceRegistrar {
	server := grpc.NewServer()
	agentgrpc.Register(server, e.Server)
	return server
}

// SeedAgent 快速创建一个代理档案。
func (e *Env) SeedAgent(tenantUUID, alias string) uuid.UUID {
	profile := &agentmodel.AgentProfileLifecycle{
		TenantUUID:  tenantUUID,
		Alias:       alias,
		DisplayName: alias,
		Status:      "pending",
	}
	if err := e.DB.Create(profile).Error; err != nil {
		e.T.Fatalf("seed agent: %v", err)
	}
	return profile.UUID
}

// MockNotifier 用于在测试中拦截企业 IM 通知。
type MockNotifier struct {
	mu       sync.Mutex
	messages []imnotify.Message
	ch       chan imnotify.Message
}

// NewMockNotifier 构造 MockNotifier。
func NewMockNotifier() *MockNotifier {
	return &MockNotifier{
		ch: make(chan imnotify.Message, 10),
	}
}

// Send 记录通知消息并推送到频道。
func (m *MockNotifier) Send(ctx context.Context, msg imnotify.Message) error {
	m.mu.Lock()
	m.messages = append(m.messages, msg)
	m.mu.Unlock()
	select {
	case m.ch <- msg:
	default:
	}
	return nil
}

// Messages 返回当前捕获的所有消息副本。
func (m *MockNotifier) Messages() []imnotify.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]imnotify.Message, len(m.messages))
	copy(out, m.messages)
	return out
}

// WaitForMessage 在指定时间内等待通知。
func (m *MockNotifier) WaitForMessage(timeout time.Duration) (imnotify.Message, bool) {
	select {
	case msg := <-m.ch:
		return msg, true
	case <-time.After(timeout):
		return imnotify.Message{}, false
	}
}

// Reset 清空已捕获的消息，便于多阶段断言。
func (m *MockNotifier) Reset() {
	m.mu.Lock()
	m.messages = nil
	m.mu.Unlock()
	for {
		select {
		case <-m.ch:
		default:
			return
		}
	}
}

// MockManifestValidator 用于控制 manifest 校验行为。
type MockManifestValidator struct {
	mu                sync.Mutex
	RequiredSignature string
	Err               error
	Calls             int
}

// NewMockManifestValidator 构造默认校验器。
func NewMockManifestValidator() *MockManifestValidator {
	return &MockManifestValidator{
		RequiredSignature: "valid-signature",
	}
}

// Validate 实现 ManifestValidator 接口。
func (m *MockManifestValidator) Validate(_ context.Context, in agent_lifecycle.ManifestRegistrationInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls++
	if m.Err != nil {
		return m.Err
	}
	if m.RequiredSignature != "" && in.Signature != m.RequiredSignature {
		return agent_lifecycle.ErrInvalidManifestSignature
	}
	return nil
}

// MockSandboxRunner 记录沙箱执行。
type MockSandboxRunner struct {
	mu        sync.Mutex
	RunInputs []agent_lifecycle.SandboxRunInput
	Result    agent_lifecycle.SandboxRunResult
	Err       error
}

// NewMockSandboxRunner 构造默认 Runner。
func NewMockSandboxRunner() *MockSandboxRunner {
	return &MockSandboxRunner{
		Result: agent_lifecycle.SandboxRunResult{
			Status: "completed",
		},
	}
}

// Run 实现 SandboxRunner 接口。
func (m *MockSandboxRunner) Run(_ context.Context, input agent_lifecycle.SandboxRunInput) (*agent_lifecycle.SandboxRunResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RunInputs = append(m.RunInputs, input)
	if m.Err != nil {
		return nil, m.Err
	}
	res := m.Result
	if res.ExecutedAt.IsZero() {
		res.ExecutedAt = time.Now().UTC()
	}
	if res.Profile == "" {
		res.Profile = input.Profile
	}
	return &res, nil
}

// LastInput 返回最近一次执行的输入。
func (m *MockSandboxRunner) LastInput() *agent_lifecycle.SandboxRunInput {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.RunInputs) == 0 {
		return nil
	}
	latest := m.RunInputs[len(m.RunInputs)-1]
	return &latest
}

// MockShareValidator 控制共享验证。
type ShareValidationCall struct {
	AgentID    uuid.UUID
	TenantUUID string
	Quotas     []agent_lifecycle.ShareQuota
	Metadata   map[string]string
}

type MockShareValidator struct {
	mu    sync.Mutex
	Err   error
	Calls []ShareValidationCall
}

func NewMockShareValidator() *MockShareValidator {
	return &MockShareValidator{}
}

func (m *MockShareValidator) Validate(_ context.Context, agent *agent_lifecycle.Agent, tenantUUID string, quotas []agent_lifecycle.ShareQuota, metadata map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	call := ShareValidationCall{
		TenantUUID: tenantUUID,
		Quotas:     append([]agent_lifecycle.ShareQuota(nil), quotas...),
	}
	if agent != nil {
		call.AgentID = agent.ID
	}
	if len(metadata) > 0 {
		call.Metadata = make(map[string]string, len(metadata))
		for k, v := range metadata {
			call.Metadata[k] = v
		}
	}
	m.Calls = append(m.Calls, call)
	return m.Err
}

// MockQuotaProvisioner 控制配额复制。
type MockQuotaProvisioner struct {
	mu             sync.Mutex
	ProvisionCalls int
	ReleaseCalls   int
	ErrProvision   error
	ErrRelease     error
}

func NewMockQuotaProvisioner() *MockQuotaProvisioner {
	return &MockQuotaProvisioner{}
}

func (m *MockQuotaProvisioner) Provision(_ context.Context, _ *agent_lifecycle.Agent, _ string, _ []agent_lifecycle.ShareQuota, _ map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ProvisionCalls++
	return m.ErrProvision
}

func (m *MockQuotaProvisioner) Release(_ context.Context, _ *agent_lifecycle.AgentShare) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ReleaseCalls++
	return m.ErrRelease
}
