package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/pluginbinding"
	auditmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	eventmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestInstallationReadinessDoesNotRepairMissingOrRevokedMappings(t *testing.T) {
	db := newPluginDrainTestDB(t)
	ctx := context.Background()
	tenant := tenantCapabilityGrantTestUUID
	require.NoError(t, db.AutoMigrate(&eventmodel.TopicDefinition{}, &eventmodel.AclBinding{}, &eventmodel.TopicManifestBinding{}, &eventmodel.AclManifestBinding{}, &auditmodel.AuditEvent{}))
	root := t.TempDir()
	path := filepath.Join(root, "events.yaml")
	require.NoError(t, os.WriteFile(path, []byte("version: v1\ntopics:\n- key: ready.one\n  namespace: fixture.one\n  name: v1\n  acl:\n  - principal_type: plugin\n    principal_id: '{{plugin_id}}'\n    actions: [publish]\n"), 0600))
	path, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	p := plugin_mgr.Plugin{ID: "com.powerx.plugins.fixture", Version: "1.0.0", Paths: plugin_mgr.InstalledPaths{Root: root}, RequiredCapabilities: []string{"com.corex.eventfabric.service_publish"}, HostConfig: &plugin_mgr.HostConfig{Spec: map[string]any{"runtime": map[string]any{"event_bridge": map[string]any{"topic_manifest": "events.yaml"}}}}}
	svc := NewTenantPluginInstanceService(db)
	status, err := svc.InstallationReadiness(ctx, tenant, p)
	require.NoError(t, err)
	require.False(t, status.Ready)
	require.Equal(t, "TENANT_PLUGIN_NOT_FOUND", status.Reason)
	cfg := dbsetting.PluginInstanceConfig{TenantUUID: tenant, PluginID: p.ID, Key: "auth.credentials", Enabled: true, Status: "enabled", ValueJSON: datatypes.JSON(`{"client_id":"fixture","config":{}}`)}
	require.NoError(t, db.Create(&cfg).Error)
	status, err = svc.InstallationReadiness(ctx, tenant, p)
	require.NoError(t, err)
	require.False(t, status.Ready)
	require.Equal(t, "EVENT_FABRIC_INSTALL_PREPARATION_REQUIRED", status.Reason)
	var count int64
	require.NoError(t, db.Model(&eventmodel.TopicDefinition{}).Count(&count).Error)
	require.Zero(t, count, "GET must not seed missing resources")
	settings := map[string]any{}
	require.NoError(t, pluginbinding.NewService(db).Prepare(ctx, p, tenant, path, true, func(tx *gorm.DB, bindings map[string]string) error {
		pluginbinding.SetBindings(settings, tenant, bindings)
		raw, _ := json.Marshal(map[string]any{"client_id": "fixture", "config": settings})
		return tx.Model(&cfg).Update("value_json", raw).Error
	}))
	status, err = svc.InstallationReadiness(ctx, tenant, p)
	require.NoError(t, err)
	require.True(t, status.Ready)
	var topic eventmodel.TopicDefinition
	require.NoError(t, db.First(&topic).Error)
	require.NoError(t, acl.NewACLService(acl.Options{DB: db}).Revoke(ctx, acl.RevokeRequest{TenantUUID: tenant, TopicUUID: topic.UUID.String(), PrincipalID: p.ID, Actions: []acl.PrincipalAction{acl.PrincipalActionPublish}}))
	status, err = svc.InstallationReadiness(ctx, tenant, p)
	require.NoError(t, err)
	require.False(t, status.Ready)
	require.Equal(t, "EVENT_FABRIC_INSTALL_ACL_REQUIRED", status.Reason)
	allowed, err := acl.NewACLService(acl.Options{DB: db}).HasPermission(ctx, tenant, topic.UUID, p.ID, "publish", time.Now())
	require.NoError(t, err)
	require.False(t, allowed, "readiness must preserve revocation")
}

func TestTenantInstallationRecoveryRejectsChangedPackageBeforeCredentials(t *testing.T) {
	db := newPluginDrainTestDB(t)
	p := plugin_mgr.Plugin{ID: "com.powerx.plugins.fixture", Version: "2.0.0", Revision: "new"}
	_, _, _, err := NewTenantPluginInstanceService(db).Enable(plugin_mgr.WithExpectedInstallation(context.Background(), "1.0.0", "old"), tenantCapabilityGrantTestUUID, p, nil)
	require.True(t, plugin_mgr.IsCode(err, plugin_mgr.CodeConflict), "%v", err)
	var count int64
	require.NoError(t, db.Model(&dbsetting.PluginInstanceConfig{}).Count(&count).Error)
	require.Zero(t, count)
}
