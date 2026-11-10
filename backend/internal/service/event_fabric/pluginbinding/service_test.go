package pluginbinding

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testTenant = "00000000-0000-0000-0000-000000000001"
const secondTenant = "00000000-0000-0000-0000-000000000002"

func bindingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	old := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = old })
	var dialector gorm.Dialector = sqlite.Open("file:" + uuid.NewString() + "?mode=memory&cache=shared")
	if dsn := os.Getenv("POWERX_EVENTFABRIC_TEST_DSN"); dsn != "" {
		coremodel.PowerXSchema = "eventbinding_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		dialector = postgres.Open(dsn)
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableForeignKeyConstraintWhenMigrating: true})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	if db.Dialector.Name() == "postgres" {
		var database string
		require.NoError(t, db.Raw("select current_database()").Scan(&database).Error)
		require.True(t, strings.HasPrefix(database, "powerx_eventfabric_test_"), "refuse non-test database")
		schema := coremodel.PowerXSchema
		require.NoError(t, db.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
		t.Cleanup(func() { require.NoError(t, db.Exec(`DROP SCHEMA "`+schema+`" CASCADE`).Error) })
		sqlDB.SetMaxOpenConns(8)
	}
	require.NoError(t, db.AutoMigrate(&model.TopicDefinition{}, &model.AclBinding{}, &model.TopicManifestBinding{}, &model.AclManifestBinding{}, &auditmodel.AuditEvent{}))
	return db
}
func bindingPlugin(t *testing.T, keys ...string) (pm.Plugin, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "service.yaml")
	writeBindingManifest(t, path, keys...)
	p := pm.Plugin{ID: "com.powerx.plugins.fixture", Version: "1.0.0", Paths: pm.InstalledPaths{Root: root}, RequiredCapabilities: []string{"com.corex.eventfabric.service_publish"}, HostConfig: &pm.HostConfig{Spec: map[string]any{"runtime": map[string]any{"event_bridge": map[string]any{"topic_manifest": "service.yaml"}}}}}
	actual, err := ManifestPath(p)
	require.NoError(t, err)
	return p, actual
}
func writeBindingManifest(t *testing.T, path string, keys ...string) {
	t.Helper()
	raw := "version: v1\ntopics:\n"
	for _, key := range keys {
		raw += "- key: " + key + "\n  namespace: fixture." + key + "\n  name: v1\n  acl:\n  - principal_type: plugin\n    principal_id: '{{plugin_id}}'\n    actions: [publish]\n"
	}
	require.NoError(t, os.WriteFile(path, []byte(raw), 0600))
}
func persistBindings(p *pm.Plugin, tenant string) func(*gorm.DB, map[string]string) error {
	return func(_ *gorm.DB, bindings map[string]string) error {
		SetBindings(p.HostConfig.Spec, tenant, bindings)
		return nil
	}
}
func TestPrepareResourcesAreAtomic(t *testing.T) {
	for _, stage := range []string{"topic_binding", "acl_binding", "config"} {
		t.Run(stage, func(t *testing.T) {
			db := bindingTestDB(t)
			p, path := bindingPlugin(t, "one", "two")
			if stage != "config" {
				table := model.TopicManifestBinding{}.TableName()
				if stage == "acl_binding" {
					table = model.AclManifestBinding{}.TableName()
				}
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fixture_fail_binding", func(tx *gorm.DB) {
					if tx.Statement.Table == table || strings.HasSuffix(table, "."+tx.Statement.Table) {
						tx.AddError(errors.New("BINDING_WRITE_FAILED"))
					}
				}))
			}
			called := false
			err := NewService(db).Prepare(context.Background(), p, testTenant, path, true, func(*gorm.DB, map[string]string) error { called = true; return errors.New("CONFIG_WRITE_FAILED") })
			require.Error(t, err)
			require.Equal(t, stage == "config", called)
			for _, record := range []any{&model.TopicDefinition{}, &model.AclBinding{}, &model.TopicManifestBinding{}, &model.AclManifestBinding{}} {
				var n int64
				require.NoError(t, db.Model(record).Count(&n).Error)
				require.Zero(t, n)
			}
		})
	}
}
func TestPrepareReadOnlyAndRevoked(t *testing.T) {
	db := bindingTestDB(t)
	p, path := bindingPlugin(t, "one")
	svc := NewService(db)
	ctx := context.Background()
	require.NoError(t, svc.Prepare(ctx, p, testTenant, path, true, persistBindings(&p, testTenant)))
	require.NoError(t, svc.Prepare(ctx, p, testTenant, path, false, func(*gorm.DB, map[string]string) error { t.Fatal("READ_ONLY_PERSIST"); return nil }))
	var topic model.TopicDefinition
	require.NoError(t, db.First(&topic).Error)
	require.Equal(t, p.ID, directory.PluginOwner(&topic))
	require.NoError(t, acl.NewACLService(acl.Options{DB: db}).Revoke(ctx, acl.RevokeRequest{TenantUUID: testTenant, TopicUUID: topic.UUID.String(), PrincipalID: p.ID, Actions: []acl.PrincipalAction{acl.PrincipalActionPublish}}))
	for _, provision := range []bool{false, true} {
		require.ErrorContains(t, svc.Prepare(ctx, p, testTenant, path, provision, persistBindings(&p, testTenant)), "ACL_REQUIRED")
	}
	allowed, err := acl.NewACLService(acl.Options{DB: db}).HasPermission(ctx, testTenant, topic.UUID, p.ID, "publish", time.Now())
	require.NoError(t, err)
	require.False(t, allowed)
}
func TestPrepareOwnerAndNamespaceIsolation(t *testing.T) {
	for _, kind := range []string{"foreign_owner", "unowned", "foreign_namespace"} {
		t.Run(kind, func(t *testing.T) {
			db := bindingTestDB(t)
			p, path := bindingPlugin(t, "one")
			want := "TOPIC_OWNER"
			if kind == "foreign_owner" {
				want = "NAMESPACE_DENIED"
			}
			if kind == "foreign_namespace" {
				raw, err := os.ReadFile(path)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(raw), "fixture.one", "scrm.messaging")), 0600))
				want = "NAMESPACE_DENIED"
			} else {
				owner := ""
				if kind == "foreign_owner" {
					owner = "com.powerx.plugins.scrm"
				}
				_, err := directory.NewDirectoryService(directory.Options{DB: db}).CreateTopic(context.Background(), directory.CreateTopicInput{TenantUUID: testTenant, Namespace: "fixture.one", Name: "v1", PluginOwner: owner})
				require.NoError(t, err)
			}
			require.ErrorContains(t, NewService(db).Prepare(context.Background(), p, testTenant, path, true, persistBindings(&p, testTenant)), want)
			var n int64
			require.NoError(t, db.Model(&model.AclBinding{}).Where("principal_id = ?", p.ID).Count(&n).Error)
			require.Zero(t, n)
		})
	}
}
func TestPrepareUpgradeReplacesOnlyGeneratedMappings(t *testing.T) {
	db := bindingTestDB(t)
	p, path := bindingPlugin(t, "old", "keep")
	svc := NewService(db)
	ctx := context.Background()
	require.NoError(t, svc.Prepare(ctx, p, testTenant, path, true, persistBindings(&p, testTenant)))
	var old model.TopicDefinition
	require.NoError(t, db.Where("namespace = ?", "fixture.old").First(&old).Error)
	require.NoError(t, db.Model(&old).Update("max_retry", 19).Error)
	writeBindingManifest(t, path, "keep", "new")
	p.Version = "2.0.0"
	require.NoError(t, svc.Prepare(ctx, p, testTenant, path, true, persistBindings(&p, testTenant)))
	bindings, err := ExplicitBindings(p)
	require.NoError(t, err)
	require.Contains(t, bindings, "new")
	require.NotContains(t, bindings, "old")
	require.NoError(t, db.First(&old, "uuid = ?", old.UUID).Error)
	require.Equal(t, 19, old.MaxRetry)
	require.Equal(t, model.TopicLifecycleActive, old.Lifecycle)
	writeBindingManifest(t, path, "old", "keep")
	p.Version = "1.0.0"
	require.NoError(t, svc.Prepare(ctx, p, testTenant, path, true, persistBindings(&p, testTenant)))
	bindings, err = ExplicitBindings(p)
	require.NoError(t, err)
	require.Contains(t, bindings, "old")
	require.NotContains(t, bindings, "new")
	Bridge(p.HostConfig.Spec)["topic_bindings"].(map[string]any)["manual"] = testTenant + ".fixture.manual.v1"
	require.ErrorContains(t, svc.Prepare(ctx, p, testTenant, path, true, persistBindings(&p, testTenant)), "BINDING_CONFLICT")
}
func TestPrepareUsesTenantConfigurationRatherThanRuntimeTenant(t *testing.T) {
	db := bindingTestDB(t)
	runtime, path := bindingPlugin(t, "one")
	svc := NewService(db)
	ctx := context.Background()
	require.NoError(t, svc.Prepare(ctx, runtime, testTenant, path, true, persistBindings(&runtime, testTenant)))
	config := map[string]any{}
	tenantPlugin := TenantPlugin(runtime, config)
	require.Empty(t, Bridge(tenantPlugin.HostConfig.Spec)["topic_bindings"])
	require.NoError(t, svc.Prepare(ctx, tenantPlugin, secondTenant, path, true, func(_ *gorm.DB, b map[string]string) error { SetBindings(config, secondTenant, b); return nil }))
	for _, full := range Bridge(config)["topic_bindings"].(map[string]any) {
		require.Contains(t, full, secondTenant+".")
	}
	invalid := TenantPlugin(runtime, config)
	require.ErrorContains(t, svc.Prepare(ctx, invalid, testTenant, path, true, persistBindings(&invalid, testTenant)), "TENANT_MISMATCH")
}
func TestExplicitActionsDoNotRequireBothPermissionsOnEveryTopic(t *testing.T) {
	p, _ := bindingPlugin(t, "one")
	p.RequiredCapabilities = append(p.RequiredCapabilities, "com.corex.eventfabric.service_consume")
	Bridge(p.HostConfig.Spec)["topic_actions"] = map[string]any{"out": []any{"publish"}, "in": []any{"subscribe"}}
	actions, err := ExplicitActions(p, map[string]string{"out": "x", "in": "y"})
	require.NoError(t, err)
	require.Equal(t, []string{"publish"}, actions["out"])
	require.Equal(t, []string{"subscribe"}, actions["in"])
	delete(Bridge(p.HostConfig.Spec), "topic_actions")
	_, err = ExplicitActions(p, map[string]string{"out": "x"})
	require.ErrorContains(t, err, "TOPIC_ACTIONS_REQUIRED")
}

func TestPrepareCoreNamespaceAssignment(t *testing.T) {
	db := bindingTestDB(t)
	p, path := bindingPlugin(t, "one")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(raw), "fixture.one", "powerx.channel.master")), 0600))
	svc := NewService(db).WithNamespaceOwners(map[string]string{"powerx.channel.master": p.ID})
	require.NoError(t, svc.Prepare(context.Background(), p, testTenant, path, true, persistBindings(&p, testTenant)))
	var topic model.TopicDefinition
	require.NoError(t, db.First(&topic).Error)
	require.Equal(t, p.ID, directory.PluginOwner(&topic))
}

func TestPrepareConcurrentSameTenantPostgres(t *testing.T) {
	if os.Getenv("POWERX_EVENTFABRIC_TEST_DSN") == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	db := bindingTestDB(t)
	p, path := bindingPlugin(t, "one")
	svc := NewService(db)
	done := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			local := TenantPlugin(p, nil)
			done <- svc.Prepare(context.Background(), local, testTenant, path, true, persistBindings(&local, testTenant))
		}()
	}
	for i := 0; i < 4; i++ {
		require.NoError(t, <-done)
	}
	for _, record := range []any{&model.TopicDefinition{}, &model.AclBinding{}, &model.TopicManifestBinding{}, &model.AclManifestBinding{}} {
		var n int64
		require.NoError(t, db.Model(record).Count(&n).Error)
		require.EqualValues(t, 1, n)
	}
}

// 对真实交付包只读取正式声明；不执行插件二进制、迁移或安装。
func TestPackageManifestPreparation(t *testing.T) {
	root := os.Getenv("POWERX_EVENTFABRIC_PACKAGE_DIR")
	if root == "" {
		t.Skip("requires explicit package fixture")
	}
	raw, err := os.ReadFile(filepath.Join(root, "plugin.yaml"))
	require.NoError(t, err)
	var manifest pm.Manifest
	require.NoError(t, yaml.Unmarshal(raw, &manifest))
	require.Equal(t, "com.powerx.plugins.ai-craft", manifest.ID)
	// 分文件清单只读合并，避免使用会写回 plugin.yaml 的安装 Loader。
	var catalog struct {
		Capabilities pm.HostCapabilitySpec `yaml:"capabilities"`
	}
	catalogRaw, err := os.ReadFile(filepath.Join(root, manifest.Catalogs.Capabilities))
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(catalogRaw, &catalog))
	manifest.Capabilities = catalog.Capabilities

	raw, err = os.ReadFile(filepath.Join(root, "config/values.example.yaml"))
	require.NoError(t, err)
	var spec map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &spec))
	p := pm.Plugin{ID: manifest.ID, Version: manifest.Version, RequiredCapabilities: manifest.Capabilities.Required, Paths: pm.InstalledPaths{Root: root}, HostConfig: &pm.HostConfig{Spec: spec}}
	path, err := ManifestPath(p)
	require.NoError(t, err)
	db := bindingTestDB(t)
	// 明确的受控夹具归属，不修改 Core 实际配置，也不代表线上已批准。
	svc := NewService(db).WithNamespaceOwners(map[string]string{"powerx.channel.master": p.ID})
	require.NoError(t, svc.Prepare(context.Background(), p, testTenant, path, true, persistBindings(&p, testTenant)))
	require.NoError(t, svc.Prepare(context.Background(), p, testTenant, path, false, nil))
	bindings, err := ExplicitBindings(p)
	require.NoError(t, err)
	require.Len(t, bindings, 13)
	var n int64
	require.NoError(t, db.Model(&model.TopicDefinition{}).Count(&n).Error)
	require.EqualValues(t, 13, n)
	t.Logf("package=%s version=%s formal_mappings=%d", p.ID, p.Version, len(bindings))
}

func TestPrepareRetriesSameKeyRenameAfterFailedVersionStartup(t *testing.T) {
	db := bindingTestDB(t)
	p, path := bindingPlugin(t, "same")
	svc := NewService(db)
	ctx := context.Background()
	require.NoError(t, svc.Prepare(ctx, p, testTenant, path, true, persistBindings(&p, testTenant)))
	originalJSON, err := json.Marshal(p.HostConfig.Spec)
	require.NoError(t, err)
	originalRaw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(originalRaw), "fixture.same", "fixture.renamed")), 0600))
	require.NoError(t, svc.Prepare(ctx, p, testTenant, path, true, persistBindings(&p, testTenant)))
	// 新版本准备已提交、进程启动失败，安装器恢复旧 HostConfig；第二次升级仍应识别旧映射归属。
	var oldSpec map[string]any
	require.NoError(t, json.Unmarshal(originalJSON, &oldSpec))
	p.HostConfig.Spec = oldSpec
	require.NoError(t, svc.Prepare(ctx, p, testTenant, path, true, persistBindings(&p, testTenant)))
	require.Equal(t, testTenant+".fixture.renamed.v1", Bridge(p.HostConfig.Spec)["topic_bindings"].(map[string]any)["same"])
	var n int64
	require.NoError(t, db.Model(&model.TopicDefinition{}).Count(&n).Error)
	require.EqualValues(t, 2, n)
}
