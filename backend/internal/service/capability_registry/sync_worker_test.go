package capability_registry

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	eventbus "github.com/ArtisanCloud/PowerX/internal/event_bus"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/event_bus"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSyncWorkerProcessArtifact(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	bus := event_bus.NewLocalEventBus()
	events := make(chan event_bus.Event, 2)
	unsub := bus.Subscribe(eventbus.TopicCapabilityCatalogSyncSucceeded, func(evt event_bus.Event) error {
		events <- evt
		return nil
	})
	t.Cleanup(unsub)

	worker := NewSyncWorker(SyncWorkerConfig{
		DB:       db,
		EventBus: bus,
		Clock: func() time.Time {
			return time.Unix(1700000000, 0).UTC()
		},
	})

	root := buildSamplePlugin(t, true)
	require.NoError(t, worker.ProcessArtifact(ctx, root))

	recordRepo := repo.NewCapabilityRecordRepository(db, nil)
	record, err := recordRepo.GetByCapabilityID(ctx, "demo.capability")
	require.NoError(t, err)
	require.Equal(t, "demo.plugin", record.PluginID)
	require.Equal(t, "Demo Capability", record.Title)
	require.NotEmpty(t, record.CapabilitiesHash)
	require.Equal(t, "published", record.Status)

	jobRepo := repo.NewCapabilitySyncJobRepository(db)
	jobs, err := jobRepo.List(ctx, repo.CapabilitySyncJobFilter{})
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, "succeeded", jobs[0].Status)
	require.Equal(t, record.CapabilitiesHash, jobs[0].HashAfter)

	templateRepo := repo.NewWorkflowTemplateRepository(db)
	templates, err := templateRepo.ListByCapabilityID(ctx, "demo.capability")
	require.NoError(t, err)
	require.Len(t, templates, 1)
	require.Equal(t, "wf.demo", templates[0].TemplateID)

	select {
	case evt := <-events:
		payload, ok := evt.Payload.(CatalogSyncEvent)
		require.True(t, ok)
		require.Equal(t, "demo.capability", payload.CapabilityID)
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected capability.catalog.sync_succeeded event")
	}
}

func TestSyncWorkerProcessArtifactCreatesTenantRegistration(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	worker := NewSyncWorker(SyncWorkerConfig{
		DB:         db,
		TenantUUID: "tenant-corex",
		Clock: func() time.Time {
			return time.Unix(1700000000, 0).UTC()
		},
	})

	root := buildSamplePlugin(t, true)
	require.NoError(t, worker.ProcessArtifact(ctx, root))

	regRepo := repo.NewCapabilityRegistryRepository(db)
	reg, err := regRepo.GetLatest(ctx, nil, "demo.capability", "tenant-corex")
	require.NoError(t, err)
	require.Equal(t, "published", reg.Status)
	require.Len(t, reg.Adapters, 1)
	require.Equal(t, "mcp", reg.Adapters[0].TransportType)
	require.Equal(t, "demo.capability.demo-tool", reg.Adapters[0].AdapterID)
}

func TestRegistrationAdapterFromProtocolPrefixesPluginProxyForREST(t *testing.T) {
	adapter := registrationAdapterFromProtocol("demo.capability", "demo.plugin", models.ProtocolBinding{
		Channel:  "rest",
		Method:   "POST",
		Endpoint: "/api/v1/templates",
	})

	require.Equal(t, "rest", adapter.TransportType)
	require.Equal(t, "/_p/demo.plugin/api/v1/templates", adapter.Endpoint)
	require.Equal(t, "POST", adapter.Labels["method"])

	absolute := registrationAdapterFromProtocol("demo.capability", "demo.plugin", models.ProtocolBinding{
		Channel:  "rest",
		Method:   "POST",
		Endpoint: "http://127.0.0.1:18080/api/v1/templates",
	})
	require.Equal(t, "http://127.0.0.1:18080/api/v1/templates", absolute.Endpoint)
}

func TestSyncWorkerProcessArtifactMissingSchema(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	alerting := &fakeCapabilityAlerting{}
	worker := NewSyncWorker(SyncWorkerConfig{
		DB:       db,
		Alerting: alerting,
	})

	root := buildSamplePlugin(t, false)
	err := worker.ProcessArtifact(ctx, root)
	require.Error(t, err)
	var alertErr *AssetAlertError
	require.True(t, errors.As(err, &alertErr))
	require.Equal(t, AssetAlertReasonSchemaMissing, alertErr.Reason)
	require.Equal(t, "demo.capability", alertErr.CapabilityID)
	require.Len(t, alerting.events, 1)
	require.Equal(t, "contracts/exposure/mcp-tools.json", alerting.events[0].AssetPath)

	jobRepo := repo.NewCapabilitySyncJobRepository(db)
	jobs, err := jobRepo.List(ctx, repo.CapabilitySyncJobFilter{})
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	require.Equal(t, "failed", jobs[0].Status)
}

func TestSyncWorkerProcessArtifactMissingExposureDir(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	alerting := &fakeCapabilityAlerting{}
	worker := NewSyncWorker(SyncWorkerConfig{
		DB:       db,
		Alerting: alerting,
	})

	root := buildSamplePlugin(t, true)
	require.NoError(t, os.RemoveAll(filepath.Join(root, "contracts")))

	err := worker.ProcessArtifact(ctx, root)
	require.Error(t, err)
	var alertErr *AssetAlertError
	require.True(t, errors.As(err, &alertErr))
	require.Equal(t, AssetAlertReasonExposureMissing, alertErr.Reason)
	require.Len(t, alerting.events, 1)
	require.Equal(t, "contracts/exposure", alerting.events[0].AssetPath)
}

func newMemoryDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.Exec("PRAGMA foreign_keys = ON").Error)

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = prevSchema
	})

	require.NoError(t, db.AutoMigrate(
		&models.CapabilityRecord{},
		&models.CapabilityRegistration{},
		&models.AdapterEndpoint{},
		&models.RoutingPolicy{},
		&models.FallbackPlan{},
		&models.WorkflowTemplateRef{},
		&models.CapabilitySyncJob{},
	))
	return db
}

func buildSamplePlugin(t *testing.T, includeSchema bool) string {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.yaml"), []byte(`
id: demo.plugin
name: Demo Plugin
version: "1.0.0"
runtime:
  type: golang
  entry: ./cmd/app/main.go
`), 0o644))

	contractsDir := filepath.Join(root, "contracts", "exposure")
	require.NoError(t, os.MkdirAll(contractsDir, 0o755))
	if includeSchema {
		require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "mcp-tools.json"), []byte(`{}`), 0o644))
	}

	capDir := filepath.Join(root, "capabilities")
	require.NoError(t, os.MkdirAll(capDir, 0o755))

	catalog := map[string]interface{}{
		"plugin": map[string]string{
			"id":      "demo.plugin",
			"version": "1.0.0",
			"name":    "Demo Plugin",
		},
		"capabilities": []map[string]interface{}{
			{
				"id":          "demo.capability",
				"title":       "Demo Capability",
				"description": "Capability provided by demo plugin",
				"intents":     []string{"demo.intent"},
				"tool_scope":  []string{"global"},
				"policy": map[string]interface{}{
					"prefer": "mcp",
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
						"template_id": "wf.demo",
						"name":        "Workflow Demo",
						"steps": []map[string]interface{}{
							{"id": "step1", "type": "task"},
						},
					},
				},
			},
		},
	}
	raw, err := json.MarshalIndent(catalog, "", "  ")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(capDir, "catalog.json"), raw, 0o644))

	return root
}

type fakeCapabilityAlerting struct {
	events []AssetAlertInput
}

func (f *fakeCapabilityAlerting) NotifyAssetIssue(ctx context.Context, input AssetAlertInput) {
	f.events = append(f.events, input)
}
