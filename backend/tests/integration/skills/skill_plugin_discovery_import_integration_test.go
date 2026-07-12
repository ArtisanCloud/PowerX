package skillsintegration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/stretchr/testify/require"
)

func TestPluginSkillDiscoveryImportsPublishedAndRejectsInvalidManifest(t *testing.T) {
	db := setupSkillsDB(t)
	registryRepo := skillrepo.NewSkillRegistryRepository(db)
	bindingRepo := skillrepo.NewSkillCapabilityBindingRepository(db)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(db)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(db)
	importSvc := skillservice.NewImportService(registryRepo, skillservice.NewAuditTraceService(traceRepo, auditRepo)).
		WithCapabilityBindingRepository(bindingRepo)

	pluginServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/plugin/skills", r.URL.Path)
		require.Equal(t, "Bearer delegated-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{
				pluginManifestFixture("mediax.video_rebuilder.cn", "com.powerx.plugin.mediax-studio"),
			},
		})
	}))
	defer pluginServer.Close()

	discoverySvc := skillservice.NewPluginSkillDiscoveryService(importSvc)
	imported, err := discoverySvc.DiscoverAndImport(context.Background(), skillservice.DiscoverPluginSkillsInput{
		ProviderPluginID: "com.powerx.plugin.mediax-studio",
		BaseURL:          pluginServer.URL,
		BearerToken:      "delegated-token",
		Operator:         "plugin-installer",
	})
	require.NoError(t, err)
	require.Len(t, imported, 1)
	require.Equal(t, skillmodel.SkillStatusPublished, imported[0].Status)
	require.True(t, imported[0].IsLatestPublished)
	require.Equal(t, skillmodel.SkillSourcePlugin, imported[0].Source)

	saved, err := registryRepo.GetBySkillVersion(context.Background(), "mediax.video_rebuilder.cn", "1.0.0")
	require.NoError(t, err)
	require.Equal(t, pluginServer.URL, saved.SourceURL)
	require.Equal(t, skillmodel.SkillStatusPublished, saved.Status)

	bindings, err := bindingRepo.ListBySkillVersion(context.Background(), "mediax.video_rebuilder.cn", "1.0.0")
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, "creation.video_automation.ingest", bindings[0].CapabilityID)
	require.Equal(t, "active", bindings[0].BindingStatus)

	invalidServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"items": []map[string]interface{}{
				{
					"skill_id":    "broken.skill",
					"provider":    "com.powerx.plugin.mediax-studio",
					"version":     "1.0.0",
					"description": "broken",
					"executor": map[string]interface{}{
						"type":       "plugin_http",
						"method":     "POST",
						"path":       "/api/v1/plugin/broken",
						"capability": "broken.invoke",
					},
				},
			},
		})
	}))
	defer invalidServer.Close()

	_, err = discoverySvc.DiscoverAndImport(context.Background(), skillservice.DiscoverPluginSkillsInput{
		ProviderPluginID: "com.powerx.plugin.mediax-studio",
		BaseURL:          invalidServer.URL,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "input_schema is required")
}

func pluginManifestFixture(skillID, provider string) map[string]interface{} {
	return map[string]interface{}{
		"skill_id":        skillID,
		"provider":        provider,
		"version":         "1.0.0",
		"title":           "视频智能重构",
		"description":     "MediaX video rebuilder skill",
		"intent_examples": []string{"帮我重构这个 shorts"},
		"input_schema": map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"urls": map[string]interface{}{"type": "array"}},
		},
		"executor": map[string]interface{}{
			"type":       "plugin_http",
			"method":     "POST",
			"path":       "/api/v1/creation/video-automation/ingest",
			"capability": "creation.video_automation.ingest",
		},
	}
}
