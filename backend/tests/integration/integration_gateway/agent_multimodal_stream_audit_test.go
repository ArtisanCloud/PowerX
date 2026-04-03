//go:build integration

package integrationgatewayintegration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	registry "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	agenthttp "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/agent"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	flowschema "github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/utils/testutil"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"regexp"
)

func TestAgentStreamSSEAuditAndTenantIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := newAgentStreamDB(t)
	tenantUUID := "tenant-agent-001"
	otherTenant := "tenant-agent-002"

	agentID := seedAgentFixtures(t, db, tenantUUID)
	traceRepo := caprepo.NewInvocationTraceRepository(db)
	auditSvc := capservice.NewAuditService(capservice.AuditServiceOptions{
		TraceRepo: traceRepo,
	})

	deps := &shared.Deps{
		DB:                      db,
		CapabilityRegistryAudit: auditSvc,
	}

	engine := gin.New()
	group := engine.Group("/api")
	agenthttp.RegisterAPIRoutes(group, group, deps)

	registerMockAgent(t)

	traceID := "trace-agent-stream"
	req := httptest.NewRequest(http.MethodGet, "/api/agents/stream/sse?q=hello&agent_id="+strconv.FormatUint(agentID, 10)+"&env=default", nil)
	req = withTenantContext(req, tenantUUID, traceID)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	_, err := traceRepo.GetByTraceID(context.Background(), traceID)
	require.NoError(t, err, "audit trace should be recorded for agent stream")

	// tenant isolation: same agent id but different tenant should be rejected
	denyReq := httptest.NewRequest(http.MethodGet, "/api/agents/stream/sse?q=hello&agent_id="+strconv.FormatUint(agentID, 10)+"&env=default", nil)
	denyReq = withTenantContext(denyReq, otherTenant, "trace-agent-deny")
	denyRec := httptest.NewRecorder()
	engine.ServeHTTP(denyRec, denyReq)
	require.Equal(t, http.StatusNotFound, denyRec.Code)
	_, err = traceRepo.GetByTraceID(context.Background(), "trace-agent-deny")
	require.Error(t, err, "tenant-isolated request should not write audit trace")
}

func TestMultimodalSessionStreamAuditAndTenantIsolation(t *testing.T) {
	db := newInvocationDB(t)

	recordRepo := caprepo.NewCapabilityRecordRepository(db, nil)
	templateRepo := caprepo.NewWorkflowTemplateRepository(db)
	jobRepo := caprepo.NewCapabilitySyncJobRepository(db)

	catalog := capservice.NewRegistryService(capservice.RegistryServiceOptions{
		DB:           db,
		RecordRepo:   recordRepo,
		TemplateRepo: templateRepo,
		JobRepo:      jobRepo,
	})

	const (
		capabilityID = "com.corex.ai.llm.stream"
		tenantUUID   = "tenant-allowed"
		modelKey     = "model-ok"
	)

	insertCapabilityRecord(t, recordRepo, capabilityID)

	routerSvc := router.NewService(router.ServiceOptions{
		RegistryRepository: stubRegistryRepo{
			reg: registry.Registration{
				CapabilityID: capabilityID,
				TenantUUID:   tenantUUID,
				Status:       "published",
				Adapters: []registry.AdapterEndpoint{
					{
						AdapterID:     "adapter-rest",
						TransportType: "rest",
						Endpoint:      "/api/v1/ai/llm/stream",
						Labels: map[string]string{
							"method": "POST",
						},
						IsActive: true,
					},
				},
				RoutingPolicy: registry.RoutingPolicy{Strategy: "default"},
				Version:       1,
			},
		},
	})

	traceRepo := caprepo.NewInvocationTraceRepository(db)
	auditSvc := capservice.NewAuditService(capservice.AuditServiceOptions{
		TraceRepo: traceRepo,
	})

	testutil.SkipIfNoLocalListener(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/ai/llm/stream" {
			http.NotFound(w, r)
			return
		}
		payload := map[string]any{"ok": true, "session_id": "session-001"}
		data, _ := json.Marshal(payload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
	}))
	defer server.Close()

	invoker := capservice.NewInvocationService(capservice.InvocationServiceOptions{
		Catalog:       catalog,
		Router:        routerSvc,
		TraceRepo:     traceRepo,
		Audit:         auditSvc,
		HTTPBaseURL:   server.URL,
		ModelVerifier: allowModelVerifier{allowedTenant: tenantUUID, allowedModelKey: modelKey},
	})

	traceID := "trace-mm-stream"
	_, err := invoker.Invoke(context.Background(), capservice.InvocationInput{
		CapabilityID:      capabilityID,
		TenantUUID:        tenantUUID,
		PreferredProtocol: "rest",
		TraceID:           traceID,
		Payload: map[string]any{
			"method":    "POST",
			"endpoint":  "/api/v1/ai/llm/stream",
			"model_key": modelKey,
			"modality":  "llm",
		},
		Context: map[string]any{
			"env": "default",
		},
	})
	require.NoError(t, err)
	_, err = traceRepo.GetByTraceID(context.Background(), traceID)
	require.NoError(t, err, "audit trace should be recorded for multimodal stream")

	// tenant isolation: invalid tenant should be rejected by model verifier
	_, err = invoker.Invoke(context.Background(), capservice.InvocationInput{
		CapabilityID: capabilityID,
		TenantUUID:   "tenant-denied",
		Payload: map[string]any{
			"method":    "POST",
			"endpoint":  "/api/v1/ai/llm/stream",
			"model_key": modelKey,
			"modality":  "llm",
		},
		Context: map[string]any{
			"env": "default",
		},
	})
	require.Error(t, err)
	_, err = traceRepo.GetByTraceID(context.Background(), "trace-denied")
	require.Error(t, err)
}

func newAgentStreamDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, createAgentStreamTables(db))
	return db
}

func newInvocationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&models.CapabilityRecord{},
		&models.WorkflowTemplateRef{},
		&models.CapabilitySyncJob{},
		&models.InvocationTrace{},
	))
	return db
}

func createAgentStreamTables(db *gorm.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS agents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT,
			env TEXT,
			tenant_uuid TEXT,
			key TEXT,
			name TEXT,
			description TEXT,
			source TEXT,
			scope TEXT,
			visibility TEXT,
			status TEXT,
			default_persona_id INTEGER,
			blueprint_refs TEXT,
			intent_cards_ref TEXT,
			tool_allowlist TEXT,
			session_singleton INTEGER,
			default_ttl_days INTEGER,
			default_max_kb INTEGER,
			default_max_tokens INTEGER,
			kb_strategy TEXT,
			meta TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS agent_settings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			env TEXT,
			tenant_uuid TEXT,
			agent_id INTEGER,
			provider TEXT,
			model TEXT,
			params TEXT,
			override_flags TEXT,
			quota_policy TEXT,
			health_status TEXT,
			health_info TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS agent_chat_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			env TEXT,
			tenant_uuid TEXT,
			agent_id INTEGER,
			user_id INTEGER,
			title TEXT,
			singleton INTEGER,
			ttl_days INTEGER,
			max_kb INTEGER,
			max_tokens INTEGER,
			summary TEXT,
			summary_at DATETIME,
			status TEXT,
			latest_at DATETIME,
			expired_at DATETIME,
			meta TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS agent_chat_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			env TEXT,
			tenant_uuid TEXT,
			session_id INTEGER,
			agent_id INTEGER,
			role TEXT,
			content TEXT,
			content_type TEXT,
			format TEXT,
			tokens INTEGER,
			size_bytes INTEGER,
			pinned INTEGER,
			is_error INTEGER,
			meta TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS ai_provider_credentials (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			env TEXT,
			tenant_uuid TEXT,
			name TEXT,
			provider TEXT,
			auth_scheme TEXT,
			data TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS ai_model_profiles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			env TEXT,
			tenant_uuid TEXT,
			modality TEXT,
			provider TEXT,
			model TEXT,
			label TEXT,
			defaults TEXT,
			cap_cache TEXT,
			tags TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS ai_route_policies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			env TEXT,
			tenant_uuid TEXT,
			modality TEXT,
			agent_id TEXT,
			flow_id TEXT,
			purpose TEXT,
			provider TEXT,
			model TEXT,
			strategy TEXT,
			compliance TEXT,
			quota TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS capability_registry_invocation_traces (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uuid TEXT,
			trace_id TEXT,
			tenant_uuid TEXT,
			plugin_id TEXT,
			capability_id TEXT,
			route_id TEXT,
			preferred_protocol TEXT,
			protocol_used TEXT,
			fallback_used INTEGER,
			fallback_reason TEXT,
			status TEXT,
			idempotency_key TEXT,
			request_payload TEXT,
			response_payload TEXT,
			error_summary TEXT,
			latency_ms INTEGER,
			event_published INTEGER,
			event_publication_id TEXT,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		);`,
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func seedAgentFixtures(t *testing.T, db *gorm.DB, tenantUUID string) uint64 {
	t.Helper()

	env := "default"
	tenantPtr := &tenantUUID

	profile := &agentmodel.AIModelProfile{
		Env:        env,
		TenantUUID: tenantPtr,
		Modality:   "llm",
		Provider:   "mock",
		Model:      "mock-model",
		Label:      "mock-profile",
		Defaults:   datatypes.JSONMap{"temperature": 0.1, "maxTokens": 32, "stream": true},
	}
	require.NoError(t, db.Create(profile).Error)

	cred := &agentmodel.AIProviderCredential{
		Env:        env,
		TenantUUID: tenantPtr,
		Name:       slug(env + "-mock"),
		Provider:   "mock",
		AuthScheme: "bearer",
		Data: datatypes.JSONMap{
			"base_url": "http://example.com",
			"api_key":  "unit-test",
		},
	}
	require.NoError(t, db.Create(cred).Error)

	agentRow := &agentmodel.Agent{
		Env:        env,
		TenantUUID: tenantPtr,
		Key:        "agent-test",
		Name:       "Agent Test",
		Status:     agentmodel.AgentStatusActive,
		Scope:      agentmodel.AgentScopeTenant,
	}
	require.NoError(t, db.Create(agentRow).Error)
	return agentRow.ID
}

func registerMockAgent(t *testing.T) {
	t.Helper()
	mgr := agent.GetAgentManager()
	clientID := "mock-agent-" + uuid.NewString()
	mock := &mockAgentClient{}
	meta := &agentschema.AgentMeta{
		ID:     clientID,
		Name:   "mock",
		FlowID: "mock-flow",
		Status: agentschema.StatusReady,
	}
	require.NoError(t, mgr.Register(clientID, mock, meta))
	require.NoError(t, mgr.SetDefaultAgent(clientID, "mock-flow"))
}

type mockAgentClient struct{}

func (m *mockAgentClient) GetInfo() *agentschema.AgentInfo { return &agentschema.AgentInfo{} }
func (m *mockAgentClient) ListFlows(ctx context.Context, meta agentschema.ExecutionMeta) ([]agentschema.FlowRuntimeInfo, error) {
	return []agentschema.FlowRuntimeInfo{{Flow: flowschema.Flow{FlowID: "mock-flow"}}}, nil
}
func (m *mockAgentClient) GetFlowInfo(ctx context.Context, flowID string, meta agentschema.ExecutionMeta) (*agentschema.FlowRuntimeInfo, error) {
	return &agentschema.FlowRuntimeInfo{Flow: flowschema.Flow{FlowID: flowID}}, nil
}
func (m *mockAgentClient) ValidateParams(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) error {
	return nil
}
func (m *mockAgentClient) Invoke(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
	return &agentschema.ExecutionResult{Success: true}, nil
}
func (m *mockAgentClient) InvokeAsync(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (string, error) {
	return "exec-mock", nil
}
func (m *mockAgentClient) Stream(ctx context.Context, flowID string, params flowschema.Context, meta agentschema.ExecutionMeta) (*schema.StreamReader[*agentschema.ExecutionResult], error) {
	reader, writer := schema.Pipe[*agentschema.ExecutionResult](1)
	go func() {
		defer writer.Close()
		writer.Send(&agentschema.ExecutionResult{
			Success:   true,
			Data:      map[string]any{"result": map[string]any{"content": "ok"}},
			StepID:    "step-1",
			Timestamp: time.Now().Unix(),
			Metadata: map[string]any{
				"is_final": true,
			},
		}, nil)
	}()
	return reader, nil
}
func (m *mockAgentClient) GetExecutionStatus(ctx context.Context, executionID string, meta agentschema.ExecutionMeta) (*agentschema.ExecutionStatus, error) {
	return &agentschema.ExecutionStatus{Status: "completed"}, nil
}
func (m *mockAgentClient) CancelExecution(ctx context.Context, executionID string, meta agentschema.ExecutionMeta) error {
	return nil
}
func (m *mockAgentClient) GetExecutionResult(ctx context.Context, executionID string, meta agentschema.ExecutionMeta) (*agentschema.ExecutionResult, error) {
	return &agentschema.ExecutionResult{Success: true}, nil
}
func (m *mockAgentClient) GetMetrics(ctx context.Context, meta agentschema.ExecutionMeta) (flowschema.Result, error) {
	return flowschema.Result{}, nil
}
func (m *mockAgentClient) Health(ctx context.Context) error   { return nil }
func (m *mockAgentClient) Shutdown(ctx context.Context) error { return nil }

type allowModelVerifier struct {
	allowedTenant   string
	allowedModelKey string
}

func (v allowModelVerifier) VerifyModelKey(ctx context.Context, tenantUUID, env, modality, modelKey string) error {
	if tenantUUID != v.allowedTenant || modelKey != v.allowedModelKey {
		return errors.New("model_key not allowed")
	}
	return nil
}

type stubRegistryRepo struct {
	reg registry.Registration
}

func (s stubRegistryRepo) Create(ctx context.Context, db *gorm.DB, reg registry.Registration) (registry.Registration, error) {
	return registry.Registration{}, errors.New("not implemented")
}
func (s stubRegistryRepo) Update(ctx context.Context, db *gorm.DB, reg registry.Registration, expectedVersion uint64) (registry.Registration, error) {
	return registry.Registration{}, errors.New("not implemented")
}
func (s stubRegistryRepo) Disable(ctx context.Context, db *gorm.DB, capabilityID, tenantUUID, reason, actor string, expectedVersion uint64, next registry.Registration) (registry.Registration, error) {
	return registry.Registration{}, errors.New("not implemented")
}
func (s stubRegistryRepo) GetLatest(ctx context.Context, db *gorm.DB, capabilityID, tenantUUID string) (registry.Registration, error) {
	return s.reg, nil
}
func (s stubRegistryRepo) GetVersion(ctx context.Context, db *gorm.DB, capabilityID, tenantUUID string, version uint64) (registry.Registration, error) {
	return registry.Registration{}, errors.New("not implemented")
}
func (s stubRegistryRepo) ListLatest(ctx context.Context, db *gorm.DB, tenantUUID string, limit, offset int) ([]registry.Registration, int64, error) {
	return nil, 0, errors.New("not implemented")
}

func insertCapabilityRecord(t *testing.T, repo *caprepo.CapabilityRecordRepository, capabilityID string) {
	t.Helper()
	record := &models.CapabilityRecord{
		CapabilityID:  capabilityID,
		PluginID:      "corex.platform",
		PluginVersion: "2025.01",
		Title:         "Multimodal Stream",
		Description:   "Test multimodal stream capability.",
		Policy:        datatypes.JSON([]byte(`{"prefer":"rest"}`)),
		Protocols:     datatypes.JSON([]byte(`[]`)),
		Status:        "published",
	}
	_, err := repo.Upsert(context.Background(), record)
	require.NoError(t, err)
}

func withTenantContext(req *http.Request, tenantUUID, traceID string) *http.Request {
	ctx := reqctx.WithTenantUUID(req.Context(), tenantUUID)
	ctx = reqctx.WithTraceID(ctx, traceID)
	return req.WithContext(ctx)
}

func slug(s string) string {
	s = strings.ToLower(s)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
