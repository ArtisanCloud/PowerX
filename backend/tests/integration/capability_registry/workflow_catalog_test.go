package capabilityregistryintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capmetrics "github.com/ArtisanCloud/PowerX/internal/observability/metrics"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/capability_registrydto"
	workflowengine "github.com/ArtisanCloud/PowerX/internal/workflow/engine"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelregistry "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWorkflowCatalogEndToEnd(t *testing.T) {
	env := newWorkflowIntegrationEnv(t)
	t.Cleanup(env.Close)

	artifact := buildWorkflowPluginArtifact(t, workflowPluginArtifactOptions{
		PluginID:              "demo.workflow.plugin",
		PluginVersion:         "1.0.0",
		CapabilityID:          "demo.workflow.capability",
		CapabilityTitle:       "Demo Workflow Capability",
		CapabilityDescription: "v1",
		TemplateID:            "tpl.demo.workflow",
		TemplateName:          "Workflow Demo",
		RequiresManualUpgrade: true,
	})
	require.NoError(t, env.worker.ProcessArtifact(env.ctx, artifact))

	engine := env.newAdminEngine()
	templates, meta := listWorkflowTemplates(t, engine)
	require.Len(t, templates, 1)

	tpl := templates[0]
	require.True(t, tpl.NeedsUpgrade)
	require.Equal(t, "demo.workflow.capability", tpl.CapabilityID)
	require.Equal(t, "demo.workflow.plugin", tpl.PluginID)

	approval := approveWorkflowTemplate(t, engine, tpl.TemplateID, tpl.CapabilitiesHash)
	require.Equal(t, tpl.CapabilitiesHash, approval.CapabilitiesHash)

	templates, _ = listWorkflowTemplates(t, engine)
	require.False(t, templates[0].NeedsUpgrade)
	require.NotNil(t, templates[0].Approved)

	snapshot, err := env.catalog.Snapshot(env.ctx)
	require.NoError(t, err)
	require.Equal(t, meta.Version, snapshot.Version)
	require.Equal(t, tpl.CapabilityTitle, snapshot.Templates[0].CapabilityTitle)

	fakeInvoker := &fakeInvocationService{}
	selector := capservice.NewSelector(capservice.SelectorOptions{
		Store: capservice.SnapshotProviderFunc(func(ctx context.Context, tenant string, grants []string) (capservice.SelectorPolicySnapshot, error) {
			return capservice.SelectorPolicySnapshot{
				TenantID:         tenant,
				CapabilitiesHash: "snapshot-demo",
				IntentMappings: map[string]map[string]string{
					"workflow.intent.demo": {"default": tpl.CapabilityID},
				},
				PreferMatrix: map[string]capservice.ProtocolPreference{
					tpl.CapabilityID: {Prefer: "workflow", Fallback: []string{"mcp"}},
				},
			}, nil
		}),
		Invoker: fakeInvoker,
	})

	adapter := workflowengine.NewCapabilityStepAdapter(selector, env.telemetry)
	resp, err := adapter.InvokeCapability(env.ctx, workflowengine.CapabilityStepInput{
		CapabilityID:      tpl.CapabilityID,
		TenantUUID:        "tenant-demo",
		Intent:            "workflow.intent.demo",
		ToolScope:         "default",
		PreferredProtocol: "workflow",
		IdempotencyKey:    "wf-req-1",
		TraceID:           "trace-demo",
		Payload:           map[string]interface{}{"foo": "bar"},
		Context:           map[string]interface{}{"extra": "ctx"},
	})
	require.NoError(t, err)
	require.Equal(t, tpl.CapabilityID, resp.CapabilityID)
	require.Equal(t, "workflow", fakeInvoker.lastInput.PreferredProtocol)
	require.Equal(t, "tenant-demo", fakeInvoker.lastInput.TenantUUID)
}

type workflowIntegrationEnv struct {
	ctx         context.Context
	db          *gorm.DB
	redisServer *miniredis.Miniredis
	redisClient redis.UniversalClient
	bus         event_bus.EventBus
	catalog     *capservice.WorkflowCatalog
	templateSvc *capservice.WorkflowTemplateService
	worker      *capservice.SyncWorker
	telemetry   *workflowengine.WorkflowTelemetry
}

func newWorkflowIntegrationEnv(t *testing.T) *workflowIntegrationEnv {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.AutoMigrate(
		&modelregistry.CapabilityRecord{},
		&modelregistry.WorkflowTemplateRef{},
		&modelregistry.WorkflowTemplateApproval{},
		&modelregistry.CapabilitySyncJob{},
	))

	redisSrv, err := miniredis.Run()
	require.NoError(t, err)
	redisClient := redis.NewClient(&redis.Options{Addr: redisSrv.Addr()})

	templateRepo := caprepo.NewWorkflowTemplateRepository(db)
	approvalRepo := caprepo.NewWorkflowTemplateApprovalRepository(db)
	capabilityRepo := caprepo.NewCapabilityRecordRepository(db, redisClient)
	telemetry := workflowengine.NewWorkflowTelemetry(capmetrics.NewCapabilityRegistryMetrics(nil))
	catalog := capservice.NewWorkflowCatalog(capservice.WorkflowCatalogOptions{
		TemplateRepo: templateRepo,
		RecordRepo:   capabilityRepo,
		Redis:        redisClient,
		Clock: func() time.Time {
			return time.Unix(1_700_000_000, 0).UTC()
		},
		Telemetry: telemetry,
	})
	templateSvc := capservice.NewWorkflowTemplateService(capservice.WorkflowTemplateServiceOptions{
		TemplateRepo: templateRepo,
		ApprovalRepo: approvalRepo,
		Clock: func() time.Time {
			return time.Unix(1_700_000_000, 0).UTC()
		},
	})

	bus := event_bus.NewLocalEventBus()
	worker := capservice.NewSyncWorker(capservice.SyncWorkerConfig{
		DB:              db,
		Redis:           redisClient,
		EventBus:        bus,
		WorkflowCatalog: catalog,
		Clock: func() time.Time {
			return time.Unix(1_700_000_000, 0).UTC()
		},
	})

	return &workflowIntegrationEnv{
		ctx:         ctx,
		db:          db,
		redisServer: redisSrv,
		redisClient: redisClient,
		bus:         bus,
		catalog:     catalog,
		templateSvc: templateSvc,
		worker:      worker,
		telemetry:   telemetry,
	}
}

func (e *workflowIntegrationEnv) Close() {
	if e.bus != nil {
		_ = e.bus.Close()
	}
	if e.redisClient != nil {
		_ = e.redisClient.Close()
	}
	if e.redisServer != nil {
		e.redisServer.Close()
	}
	if e.db != nil {
		if sqlDB, err := e.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
}

func (e *workflowIntegrationEnv) newAdminEngine() *gin.Engine {
	eng := gin.New()
	protected := eng.Group("/")
	capability_registry.RegisterAPIRoutes(nil, protected, &shared.Deps{
		WorkflowTemplateSvc: e.templateSvc,
		WorkflowCatalog:     e.catalog,
	})
	return eng
}

type templateListPayload struct {
	Version     string                                              `json:"version"`
	GeneratedAt time.Time                                           `json:"generated_at"`
	Items       []capability_registrydto.WorkflowCatalogTemplateDTO `json:"items"`
}

func listWorkflowTemplates(t *testing.T, engine *gin.Engine) ([]capability_registrydto.WorkflowCatalogTemplateDTO, templateListPayload) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/admin/workflow-templates", nil)
	engine.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)

	var resp struct {
		Data templateListPayload `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Data.Items, resp.Data
}

func approveWorkflowTemplate(t *testing.T, engine *gin.Engine, templateID, hash string) capability_registrydto.WorkflowTemplateApprovalDTO {
	t.Helper()
	payload := map[string]string{
		"capabilities_hash": hash,
		"reason":            "integration-test",
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", fmt.Sprintf("/admin/workflow-templates/%s/upgrade", templateID), bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)

	var resp struct {
		Data capability_registrydto.WorkflowTemplateApprovalDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Data
}

type workflowPluginArtifactOptions struct {
	PluginID              string
	PluginVersion         string
	CapabilityID          string
	CapabilityTitle       string
	CapabilityDescription string
	TemplateID            string
	TemplateName          string
	RequiresManualUpgrade bool
}

func buildWorkflowPluginArtifact(t *testing.T, opts workflowPluginArtifactOptions) string {
	t.Helper()
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "contracts", "exposure"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "contracts", "exposure", "mcp-tools.json"), []byte(`{"tools":[]}`), 0o644))

	pluginYAML := fmt.Sprintf("id: %s\nname: Demo Plugin\nversion: %s\nruntime:\n  type: golang\n  entry: ./cmd/app/main.go\n", opts.PluginID, opts.PluginVersion)
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.yaml"), []byte(pluginYAML), 0o644))

	catalog := map[string]interface{}{
		"plugin": map[string]string{
			"id":      opts.PluginID,
			"version": opts.PluginVersion,
			"name":    "Demo Workflow Plugin",
		},
		"capabilities": []map[string]interface{}{
			{
				"id":          opts.CapabilityID,
				"title":       opts.CapabilityTitle,
				"description": opts.CapabilityDescription,
				"intents":     []string{"workflow.intent.demo"},
				"tool_scope":  []string{"default"},
				"policy": map[string]interface{}{
					"prefer":   "workflow",
					"fallback": []string{"mcp"},
				},
				"protocols": []map[string]interface{}{
					{
						"channel":    "mcp",
						"schema_ref": "contracts/exposure/mcp-tools.json",
						"tool_ref":   "demo-tool",
					},
				},
				"workflow_templates": []map[string]interface{}{
					{
						"template_id":             opts.TemplateID,
						"name":                    opts.TemplateName,
						"description":             "workflow demo template",
						"requires_manual_upgrade": opts.RequiresManualUpgrade,
						"steps": []map[string]interface{}{
							{
								"id":            "step-1",
								"type":          "task",
								"capability_id": opts.CapabilityID,
							},
						},
						"params_schema": map[string]interface{}{
							"type": "object",
						},
						"protocol_requirements": []map[string]interface{}{
							{
								"step_id":            "step-1",
								"channel":            "workflow",
								"preferred_protocol": "workflow",
							},
						},
					},
				},
			},
		},
	}

	catalogDir := filepath.Join(root, "capabilities")
	require.NoError(t, os.MkdirAll(catalogDir, 0o755))
	raw, err := json.MarshalIndent(catalog, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(catalogDir, "catalog.json"), raw, 0o644))

	return root
}

type fakeInvocationService struct {
	lastInput capservice.InvocationInput
	err       error
}

func (f *fakeInvocationService) Invoke(ctx context.Context, in capservice.InvocationInput) (capservice.InvocationResult, error) {
	f.lastInput = in
	if f.err != nil {
		return capservice.InvocationResult{
			TraceID:      in.TraceID,
			Status:       "failed",
			ProtocolUsed: in.PreferredProtocol,
		}, f.err
	}
	return capservice.InvocationResult{
		TraceID:      in.TraceID,
		Status:       "completed",
		ProtocolUsed: in.PreferredProtocol,
		Result:       map[string]interface{}{"echo": "ok"},
	}, nil
}
