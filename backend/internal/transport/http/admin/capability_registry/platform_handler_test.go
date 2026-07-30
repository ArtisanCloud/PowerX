package capability_registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	corerepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPlatformCapabilityModulesSourceFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := newPlatformHandlerMemoryDB(t)
	recordRepo := corerepo.NewCapabilityRecordRepository(db, nil)
	svc := capservice.NewRegistryService(capservice.RegistryServiceOptions{
		RecordRepo:   recordRepo,
		TemplateRepo: corerepo.NewWorkflowTemplateRepository(db),
		JobRepo:      corerepo.NewCapabilitySyncJobRepository(db),
	})
	handler := newPlatformHandler(svc)
	ctx := context.Background()

	_, err := recordRepo.Upsert(ctx, &models.CapabilityRecord{
		CapabilityID:     "com.corex.media.assets",
		PluginID:         "corex.platform",
		PluginVersion:    "1.0.0",
		Title:            "Media Assets",
		Protocols:        datatypes.JSON([]byte(`[{"channel":"rest"}]`)),
		CapabilitiesHash: "hash-corex",
		ProtocolHash:     "protocol-corex",
		Status:           "published",
	})
	require.NoError(t, err)
	_, err = recordRepo.Upsert(ctx, &models.CapabilityRecord{
		CapabilityID:     "com.powerx.plugins.mediax.video.parse",
		PluginID:         "com.powerx.plugins.mediax-studio",
		PluginVersion:    "0.1.1",
		Title:            "Video Parse",
		Protocols:        datatypes.JSON([]byte(`[{"channel":"rest"}]`)),
		CapabilitiesHash: "hash-plugin",
		ProtocolHash:     "protocol-plugin",
		Status:           "published",
	})
	require.NoError(t, err)

	defaultBody := performPlatformModulesRequest(handler, "")
	require.Equal(t, float64(1), defaultBody["total_capabilities"])
	requireCapabilities(t, defaultBody, []string{"com.corex.media.assets"})

	allBody := performPlatformModulesRequest(handler, "source=all")
	require.Equal(t, float64(2), allBody["total_capabilities"])
	requireCapabilities(t, allBody, []string{"com.corex.media.assets", "com.powerx.plugins.mediax.video.parse"})
}

func performPlatformModulesRequest(handler *platformCapabilityHandler, rawQuery string) map[string]any {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/platform-capabilities?"+rawQuery, nil)
	req = req.WithContext(reqctx.WithIsRoot(req.Context(), true))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	handler.ListModules(c)

	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if data, ok := body["data"].(map[string]any); ok {
		return data
	}
	return body
}

func requireCapabilities(t *testing.T, body map[string]any, expected []string) {
	t.Helper()
	modules, ok := body["modules"].([]any)
	require.True(t, ok)
	seen := map[string]bool{}
	for _, rawModule := range modules {
		module, ok := rawModule.(map[string]any)
		require.True(t, ok)
		capabilities, ok := module["capabilities"].([]any)
		require.True(t, ok)
		for _, rawCapability := range capabilities {
			capability, ok := rawCapability.(map[string]any)
			require.True(t, ok)
			if capabilityID, ok := capability["capability_id"].(string); ok {
				seen[capabilityID] = true
			}
		}
	}
	for _, capabilityID := range expected {
		require.True(t, seen[capabilityID], "missing capability %s", capabilityID)
	}
	require.Len(t, seen, len(expected))
}

func newPlatformHandlerMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() {
		coremodel.PowerXSchema = prevSchema
	})

	require.NoError(t, db.AutoMigrate(
		&models.CapabilityRecord{},
		&models.WorkflowTemplateRef{},
		&models.CapabilitySyncJob{},
	))
	return db
}
