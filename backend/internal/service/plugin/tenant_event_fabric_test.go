package plugin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/pluginbinding"
	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
)

func TestTenantPluginEventPreparationCommitsWithActivationAndIsolatesTenants(t *testing.T) {
	db := newPluginDrainTestDB(t)
	require.NoError(t, db.AutoMigrate(&capmodel.CapabilityRecord{}, &capmodel.CapabilityRegistration{}, &model.TopicDefinition{}, &model.AclBinding{}, &model.TopicManifestBinding{}, &model.AclManifestBinding{}, &auditmodel.AuditEvent{}))
	const capability = "com.corex.eventfabric.service_publish"
	const other = "00000000-0000-0000-0000-000000000002"
	seedPublishedTenantCapability(t, db, tenantCapabilityGrantTestUUID, capability)
	require.NoError(t, db.Create(&capmodel.CapabilityRegistration{CapabilityID: capability, TenantUUID: other, ContractRef: "test", Status: "published", Version: 1}).Error)
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "service.yaml"), []byte("version: v1\ntopics:\n- key: fixture.changed\n  namespace: fixture.events\n  name: changed\n  acl:\n  - principal_type: plugin\n    principal_id: '{{plugin_id}}'\n    actions: [publish]\n"), 0600))
	p := pm.Plugin{ID: "com.powerx.plugins.fixture", Version: "1.0.0", Paths: pm.InstalledPaths{Root: root}, RequiredCapabilities: []string{capability}, HostConfig: &pm.HostConfig{Spec: map[string]any{"runtime": map[string]any{"event_bridge": map[string]any{"topic_manifest": "service.yaml", "tenant_uuid": "00000000-0000-0000-0000-000000000001", "topic_bindings": map[string]any{"fixture.changed": "00000000-0000-0000-0000-000000000001.fixture.events.changed"}}}}}}
	svc := NewTenantPluginInstanceService(db)
	for _, tenant := range []string{tenantCapabilityGrantTestUUID, other} {
		instance, _, _, err := svc.Enable(context.Background(), tenant, p, nil)
		require.NoError(t, err)
		require.Equal(t, tenant, pluginbinding.Bridge(instance.Config)["tenant_uuid"])
		require.Equal(t, tenant+".fixture.events.changed", pluginbinding.Bridge(instance.Config)["topic_bindings"].(map[string]any)["fixture.changed"])
		again, _, _, err := svc.Enable(context.Background(), tenant, p, nil)
		require.NoError(t, err)
		require.Equal(t, instance.Config, again.Config)
	}
	var n int64
	require.NoError(t, db.Model(&model.TopicDefinition{}).Count(&n).Error)
	require.EqualValues(t, 2, n)
	// 已启用实例的重同步失败，不提交新的实例配置或留下新版本资源。
	require.NoError(t, os.WriteFile(filepath.Join(root, "service.yaml"), []byte("version: v1\ntopics:\n- key: foreign.changed\n  namespace: scrm.messaging\n  name: changed\n  acl:\n  - principal_type: plugin\n    principal_id: '{{plugin_id}}'\n    actions: [publish]\n"), 0600))
	_, _, _, err := svc.Enable(context.Background(), other, p, nil)
	require.ErrorContains(t, err, "NAMESPACE_DENIED")
	require.NoError(t, db.Model(&model.TopicDefinition{}).Count(&n).Error)
	require.EqualValues(t, 2, n)
	var active setting.PluginInstanceConfig
	require.NoError(t, db.Where("tenant_uuid = ? AND plugin_id = ?", other, p.ID).First(&active).Error)
	require.True(t, active.Enabled)
}
