package manager

import (
	"context"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestNotificationPresentationBindingSurvivesRegistryReload(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "registry.json")
	registry := NewJSONRegistry(file)
	binding := plugin_mgr.NotificationPresentationSpec{Contract: "business_notification_card.v1", BusinessType: "sample_track", PresentationPath: "/admin/notices/{delivery_key}/presentation", PreviewPath: "/admin/notices/{delivery_key}/images/{image_key}"}
	bindingV2 := binding
	bindingV2.Contract = "business_notification_card.v2"
	bindingV2.PresentationPath += "-v2"
	bindings := []plugin_mgr.NotificationPresentationSpec{binding, bindingV2}
	descriptor := Descriptor{Manifest: plugin_mgr.Manifest{ID: "plugin.card.test", Version: "1.0.0", NotificationPresentations: bindings}}
	require.NoError(t, registry.Put(ctx, descriptor, plugin_mgr.StateEnabled))
	require.NoError(t, registry.Save(ctx))
	reloaded := NewJSONRegistry(file)
	require.NoError(t, reloaded.Load(ctx))
	current, ok := reloaded.Get(ctx, descriptor.Manifest.ID)
	require.True(t, ok)
	require.Equal(t, bindings, current.NotificationPresentations)
	version, ok := reloaded.GetVersion(ctx, descriptor.Manifest.ID, "1.0.0")
	require.True(t, ok)
	require.Equal(t, current.NotificationPresentations, version.NotificationPresentations)
}

func TestNotificationPresentationVersionsCoexistButDuplicateVersionFails(t *testing.T) {
	binding := plugin_mgr.NotificationPresentationSpec{Contract: "business_notification_card.v1", BusinessType: "sample_track", PresentationPath: "/admin/notices/{delivery_key}/presentation", PreviewPath: "/admin/notices/{delivery_key}/images/{image_key}"}
	v2 := binding
	v2.Contract = "business_notification_card.v2"
	v2.PresentationPath += "-v2"
	manifest := plugin_mgr.Manifest{ID: "plugin.card.test", Version: "1.0.0", Runtime: plugin_mgr.RuntimeSpec{Kind: plugin_mgr.RuntimeKindStatic}, NotificationPresentations: []plugin_mgr.NotificationPresentationSpec{binding, v2}}
	loader := NewFSLoader()
	require.NoError(t, loader.Validate(context.Background(), manifest, t.TempDir()))
	manifest.NotificationPresentations = append(manifest.NotificationPresentations, v2)
	err := loader.Validate(context.Background(), manifest, t.TempDir())
	require.True(t, plugin_mgr.IsCode(err, plugin_mgr.CodeInvalidManifest))
	var validation *plugin_mgr.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Contains(t, validation.Fields, plugin_mgr.FieldError{Field: "notification_presentations", Reason: "duplicate business_type and contract"})
}
