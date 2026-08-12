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
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
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
	var annotations map[string]any
	require.NoError(t, json.Unmarshal(record.Annotations, &annotations))
	require.Equal(t, map[string]any{"zh-CN": "演示能力", "en": "Demo Capability"}, annotations["title_i18n"])
	require.Equal(t, map[string]any{"zh-CN": "演示插件提供的能力", "en": "Capability provided by demo plugin"}, annotations["description_i18n"])

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

func TestSyncWorkerReadsDescriptorSecurityForAgentGrants(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	worker := NewSyncWorker(SyncWorkerConfig{DB: db})

	root := buildDescriptorPlugin(t)
	require.NoError(t, worker.ProcessArtifact(ctx, root))

	recordRepo := repo.NewCapabilityRecordRepository(db, nil)
	record, err := recordRepo.GetByCapabilityID(ctx, "demo.descriptor.capability")
	require.NoError(t, err)

	var annotations map[string]any
	require.NoError(t, json.Unmarshal(record.Annotations, &annotations))
	require.Equal(t, "demo.template:create", annotations["permission_code"])
	require.Equal(t, true, annotations["agent_usable"])
	require.Equal(t, "medium", annotations["risk_level"])
}

func TestSyncWorkerDerivesPermissionCodeFromMergedExposureWhenDescriptorMissing(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	worker := NewSyncWorker(SyncWorkerConfig{DB: db})

	root := buildExposureFallbackPlugin(t)
	require.NoError(t, worker.ProcessArtifact(ctx, root))

	recordRepo := repo.NewCapabilityRecordRepository(db, nil)
	record, err := recordRepo.GetByCapabilityID(ctx, "demo.fallback.template.create")
	require.NoError(t, err)

	var annotations map[string]any
	require.NoError(t, json.Unmarshal(record.Annotations, &annotations))
	require.Equal(t, true, annotations["descriptor_missing"])
	require.Equal(t, []any{"demo.plugin.template:create"}, annotations["permission_codes"])
}

func TestSyncWorkerRegistersCapabilityPermissionCodesToIAM(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	worker := NewSyncWorker(SyncWorkerConfig{DB: db})

	root := buildExposureFallbackPlugin(t)
	require.NoError(t, worker.ProcessArtifact(ctx, root))

	var perm iammodel.Permission
	require.NoError(t, db.WithContext(ctx).
		Where("module = ? AND resource = ? AND action = ?", "demo.plugin", "template", "create").
		First(&perm).Error)
	require.Equal(t, "plugin:demo.plugin", perm.Source)
	require.Equal(t, iammodel.PermissionStatusActive, perm.Status)
	require.False(t, perm.AllowAPIKey)
}

func TestSyncWorkerGrantsCapabilityPermissionsToTenantOwnerAndAdminRoles(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	tenantUUID := "tenant-corex"
	roles := []iammodel.Role{
		{Scope: "tenant", TenantUUID: tenantUUID, Code: "role_owner", Name: "Tenant Owner", Builtin: true},
		{Scope: "tenant", TenantUUID: tenantUUID, Code: "role_admin", Name: "Tenant Admin", Builtin: true},
		{Scope: "tenant", TenantUUID: tenantUUID, Code: "role_user", Name: "Tenant User", Builtin: true},
	}
	require.NoError(t, db.Create(&roles).Error)

	worker := NewSyncWorker(SyncWorkerConfig{DB: db, TenantUUID: tenantUUID})
	root := buildExposureFallbackPlugin(t)
	require.NoError(t, worker.ProcessArtifact(ctx, root))

	var perm iammodel.Permission
	require.NoError(t, db.WithContext(ctx).
		Where("module = ? AND resource = ? AND action = ?", "demo.plugin", "template", "create").
		First(&perm).Error)

	countRolePermission := func(roleID uint64) int64 {
		var count int64
		require.NoError(t, db.WithContext(ctx).
			Model(&iammodel.RolePermission{}).
			Where("role_id = ? AND permission_id = ?", roleID, perm.ID).
			Count(&count).Error)
		return count
	}
	require.Equal(t, int64(1), countRolePermission(roles[0].ID))
	require.Equal(t, int64(1), countRolePermission(roles[1].ID))
	require.Equal(t, int64(0), countRolePermission(roles[2].ID))
}

func TestSyncWorkerGrantsCapabilityPermissionsToDeclaredDefaultRoles(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	tenantUUID := "tenant-corex"
	roles := []iammodel.Role{
		{Scope: "tenant", TenantUUID: tenantUUID, Code: "role_owner", Name: "Tenant Owner", Builtin: true},
		{Scope: "tenant", TenantUUID: tenantUUID, Code: "role_admin", Name: "Tenant Admin", Builtin: true},
		{Scope: "tenant", TenantUUID: tenantUUID, Code: "role_user", Name: "Tenant User", Builtin: true},
	}
	require.NoError(t, db.Create(&roles).Error)

	worker := NewSyncWorker(SyncWorkerConfig{DB: db, TenantUUID: tenantUUID})
	root := buildDescriptorPlugin(t)
	require.NoError(t, worker.ProcessArtifact(ctx, root))

	var perm iammodel.Permission
	require.NoError(t, db.WithContext(ctx).
		Where("module = ? AND resource = ? AND action = ?", "demo", "template", "create").
		First(&perm).Error)

	var count int64
	require.NoError(t, db.WithContext(ctx).
		Model(&iammodel.RolePermission{}).
		Where("role_id = ? AND permission_id = ?", roles[2].ID, perm.ID).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestSyncWorkerRegistersPluginPermissionDeclarationsToIAM(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	tenantUUID := "tenant-corex"
	roles := []iammodel.Role{
		{Scope: "tenant", TenantUUID: tenantUUID, Code: "role_owner", Name: "Tenant Owner", Builtin: true},
		{Scope: "tenant", TenantUUID: tenantUUID, Code: "role_admin", Name: "Tenant Admin", Builtin: true},
		{Scope: "tenant", TenantUUID: tenantUUID, Code: "role_user", Name: "Tenant User", Builtin: true},
	}
	require.NoError(t, db.Create(&roles).Error)

	worker := NewSyncWorker(SyncWorkerConfig{DB: db, TenantUUID: tenantUUID})
	root := buildPermissionDescriptorPlugin(t, "")
	require.NoError(t, worker.ProcessArtifact(ctx, root))

	recordRepo := repo.NewCapabilityRecordRepository(db, nil)
	record, err := recordRepo.GetByCapabilityID(ctx, "demo.permissions.capability")
	require.NoError(t, err)
	var annotations map[string]any
	require.NoError(t, json.Unmarshal(record.Annotations, &annotations))
	require.Len(t, annotations["permissions"], 2)

	var actionPerm iammodel.Permission
	require.NoError(t, db.WithContext(ctx).
		Where("module = ? AND resource = ? AND action = ?", "production", "sample_track", "factory_schedule").
		First(&actionPerm).Error)
	require.Equal(t, "plugin:demo.plugin", actionPerm.Source)
	require.Equal(t, iammodel.PermissionStatusActive, actionPerm.Status)

	var meta map[string]any
	require.NoError(t, json.Unmarshal(actionPerm.Meta, &meta))
	require.Equal(t, "action", meta["type"])
	require.Equal(t, "demo.plugin", meta["plugin_id"])
	require.Equal(t, "demo.permissions.capability", meta["capability_id"])
	require.Equal(t, "medium", meta["risk_level"])
	require.Equal(t, "sample_track", meta["resource"])
	require.Equal(t, "factory_schedule", meta["action"])
	require.Equal(t, map[string]any{"zh-CN": "小样打样排产", "en": "Sample schedule"}, meta["title_i18n"])

	var count int64
	require.NoError(t, db.WithContext(ctx).
		Model(&iammodel.RolePermission{}).
		Where("role_id = ? AND permission_id = ?", roles[2].ID, actionPerm.ID).
		Count(&count).Error)
	require.Equal(t, int64(1), count)

	var apiPerm iammodel.Permission
	require.NoError(t, db.WithContext(ctx).
		Where("module = ? AND resource = ? AND action = ?", "production", "sample_track_api", "sample_schedule").
		First(&apiPerm).Error)
	require.Equal(t, "plugin:demo.plugin", apiPerm.Source)
	require.NoError(t, json.Unmarshal(apiPerm.Meta, &meta))
	require.Equal(t, "api", meta["type"])
	require.Equal(t, "production.sample_track:factory_schedule", meta["business_permission_code"])
	require.Len(t, meta["protocol_bindings"], 1)
}

func TestSyncWorkerFailsInvalidPluginPermissionDeclarations(t *testing.T) {
	cases := []struct {
		name        string
		invalidCase string
		want        string
	}{
		{name: "missing permission code", invalidCase: "missing_permission_code", want: "invalid permission_code"},
		{name: "missing title i18n", invalidCase: "missing_title", want: "title_i18n missing"},
		{name: "missing rest method", invalidCase: "missing_method", want: "invalid method"},
		{name: "missing actor context", invalidCase: "missing_actor", want: "actor_context missing"},
		{name: "missing resource scope", invalidCase: "missing_resource_scope", want: "resource_scope missing"},
		{name: "missing api business action", invalidCase: "missing_business_permission", want: "api permission requires business_permission_code or independent=true"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db := newMemoryDB(t)
			worker := NewSyncWorker(SyncWorkerConfig{DB: db})

			root := buildPermissionDescriptorPlugin(t, tc.invalidCase)
			err := worker.ProcessArtifact(ctx, root)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)

			jobRepo := repo.NewCapabilitySyncJobRepository(db)
			jobs, err := jobRepo.List(ctx, repo.CapabilitySyncJobFilter{})
			require.NoError(t, err)
			require.Len(t, jobs, 1)
			require.Equal(t, "failed", jobs[0].Status)
			require.Contains(t, jobs[0].ErrorSummary, tc.want)
		})
	}
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

func TestSyncWorkerProcessArtifactAllowsDerivedRESTCapabilityWithoutExposureDir(t *testing.T) {
	ctx := context.Background()
	db := newMemoryDB(t)
	alerting := &fakeCapabilityAlerting{}
	worker := NewSyncWorker(SyncWorkerConfig{
		DB:       db,
		Alerting: alerting,
	})

	root := buildExposureFallbackPlugin(t)
	require.NoError(t, os.RemoveAll(filepath.Join(root, "contracts")))

	require.NoError(t, worker.ProcessArtifact(ctx, root))
	require.Empty(t, alerting.events)

	recordRepo := repo.NewCapabilityRecordRepository(db, nil)
	record, err := recordRepo.GetByCapabilityID(ctx, "demo.fallback.template.create")
	require.NoError(t, err)
	require.Equal(t, "demo.plugin", record.PluginID)
}

func buildExposureFallbackPlugin(t *testing.T) string {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.yaml"), []byte(`
id: demo.plugin
name: Demo Plugin
version: "1.0.0"
runtime:
  type: golang
  entry: ./cmd/app/main.go
catalogs:
  capabilities: ./plugin.d/capabilities.yaml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.merged.yaml"), []byte(`
id: demo.plugin
name: Demo Plugin
version: "1.0.0"
runtime:
  type: golang
  entry: ./cmd/app/main.go
catalogs:
  capabilities: ./plugin.d/capabilities.yaml
exposure:
  channels:
    - type: rest
      method: POST
      entrypoint: /api/v1/templates
      auth: jwt
      capability: demo.fallback.template.create
      rbac: template:create
`), 0o644))

	contractsDir := filepath.Join(root, "contracts", "exposure")
	require.NoError(t, os.MkdirAll(contractsDir, 0o755))

	pluginD := filepath.Join(root, "plugin.d")
	require.NoError(t, os.MkdirAll(pluginD, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginD, "capabilities.yaml"), []byte(`
capabilities:
  provides:
    - id: demo.fallback.template.create
      version: "1.0.0"
      descriptor: contracts/capabilities/missing.yaml
`), 0o644))

	return root
}

func buildPermissionDescriptorPlugin(t *testing.T, invalidCase string) string {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugin.yaml"), []byte(`
id: demo.plugin
name: Demo Plugin
version: "1.0.0"
runtime:
  type: golang
  entry: ./cmd/app/main.go
`), 0o644))

	pluginD := filepath.Join(root, "plugin.d")
	require.NoError(t, os.MkdirAll(pluginD, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginD, "capabilities.yaml"), []byte(`
capabilities:
  provides:
    - id: demo.permissions.capability
      version: "1.0.0"
      descriptor: capabilities/permissions.yaml
`), 0o644))

	titleI18n := `
      zh-CN: 小样打样排产
      en: Sample schedule`
	if invalidCase == "missing_title" {
		titleI18n = ` {}`
	}
	actionPermissionCode := "production.sample_track:factory_schedule"
	if invalidCase == "missing_permission_code" {
		actionPermissionCode = ""
	}
	apiMethod := "POST"
	if invalidCase == "missing_method" {
		apiMethod = ""
	}
	apiActorContext := "admin_user"
	if invalidCase == "missing_actor" {
		apiActorContext = ""
	}
	apiResourceScope := "tenant"
	if invalidCase == "missing_resource_scope" {
		apiResourceScope = ""
	}
	businessPermissionCode := "production.sample_track:factory_schedule"
	if invalidCase == "missing_business_permission" {
		businessPermissionCode = ""
	}

	capDir := filepath.Join(root, "capabilities")
	require.NoError(t, os.MkdirAll(capDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(capDir, "permissions.yaml"), []byte(`
id: demo.permissions.capability
type: tool
version: "1.0.0"
title: Permission Descriptor Capability
description: Permission descriptor capability
metadata:
  protocols:
    rest:
      path: /sample-tracks/*/nodes/sample-schedule
      method: POST
      auth_type: tenant_jwt
permissions:
  - type: action
    permission_code: `+actionPermissionCode+`
    module: production
    resource: sample_track
    action: factory_schedule
    title_i18n:`+titleI18n+`
    description_i18n:
      zh-CN: 允许提交小样打样排产节点
      en: Allows submitting sample schedule node
    risk_level: medium
    data_scope: tenant
    default_role_grants:
      - role_user
  - type: api
    permission_code: production.sample_track_api:sample_schedule
    module: production
    resource: sample_track_api
    action: sample_schedule
    title_i18n:
      zh-CN: 小样打样排产接口
      en: Sample schedule API
    description_i18n:
      zh-CN: 允许调用小样打样排产接口
      en: Allows calling sample schedule API
    risk_level: medium
    data_scope: tenant
    business_permission_code: `+businessPermissionCode+`
    protocol_bindings:
      - channel: rest
        method: `+apiMethod+`
        path: /sample-tracks/*/nodes/sample-schedule
        actor_context: `+apiActorContext+`
        resource_scope: `+apiResourceScope+`
`), 0o644))

	return root
}

func buildDescriptorPlugin(t *testing.T) string {
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
	require.NoError(t, os.WriteFile(filepath.Join(contractsDir, "input.json"), []byte(`{}`), 0o644))

	pluginD := filepath.Join(root, "plugin.d")
	require.NoError(t, os.MkdirAll(pluginD, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginD, "capabilities.yaml"), []byte(`
capabilities:
  provides:
    - id: demo.descriptor.capability
      version: "1.0.0"
      descriptor: capabilities/demo.yaml
      schemas:
        input: contracts/exposure/input.json
`), 0o644))

	capDir := filepath.Join(root, "capabilities")
	require.NoError(t, os.MkdirAll(capDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(capDir, "demo.yaml"), []byte(`
id: demo.descriptor.capability
type: tool
version: "1.0.0"
title: Descriptor Capability
description: Descriptor capability provided by demo plugin
default_role_grants:
  - role_user
security:
  permission_code: demo.template:create
agent:
  usable: true
  risk_level: medium
metadata:
  protocols:
    rest:
      path: /api/v1/templates
      method: POST
      auth_type: tenant_jwt
`), 0o644))

	return root
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
		&iammodel.Permission{},
		&iammodel.Role{},
		&iammodel.RolePermission{},
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
				"id":    "demo.capability",
				"title": "Demo Capability",
				"title_i18n": map[string]string{
					"zh-CN": "演示能力",
					"en":    "Demo Capability",
				},
				"description": "Capability provided by demo plugin",
				"description_i18n": map[string]string{
					"zh-CN": "演示插件提供的能力",
					"en":    "Capability provided by demo plugin",
				},
				"intents":    []string{"demo.intent"},
				"tool_scope": []string{"global"},
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
