//go:build ignore

package capabilityregistryintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	capability_registry_http "github.com/ArtisanCloud/PowerX/internal/transport/http/openapi/capability_registry"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestTenantInvokesPlatformMediaCapability(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db := initCapabilityDB(t)
	recordRepo := caprepo.NewCapabilityRecordRepository(db, redis.UniversalClient(nil))
	templateRepo := caprepo.NewWorkflowTemplateRepository(db)
	catalogSvc := capservice.NewRegistryService(capservice.RegistryServiceOptions{
		RecordRepo:   recordRepo,
		TemplateRepo: templateRepo,
	})

	insertPlatformMediaRecord(t, recordRepo)

	invoker := &stubCapabilityInvoker{}
	selector := capservice.NewSelector(capservice.SelectorOptions{
		Invoker: invoker,
	})

	deps := &shared.Deps{
		CapabilityCatalogSvc:    catalogSvc,
		CapabilityInvocationSvc: &capservice.InvocationService{},
		CapabilitySelector:      selector,
	}

	engine := gin.New()
	capability_registry_http.RegisterTenantRoutes(engine.Group("/api"), deps)

	const tenantUUID = "f7833a23-2cb2-47c9-a7ea-b67cda4c1e11"

	// List capabilities to ensure platform entries appear for tenants.
	listReq := httptest.NewRequest(http.MethodGet, "/api/tenant/capabilities?channel=media.assets", nil)
	listReq = withTenantContext(listReq, tenantUUID)
	listResp := httptest.NewRecorder()
	engine.ServeHTTP(listResp, listReq)

	require.Equal(t, http.StatusOK, listResp.Code)
	var listPayload map[string]interface{}
	require.NoError(t, json.Unmarshal(listResp.Body.Bytes(), &listPayload))
	items, ok := listPayload["items"].([]interface{})
	require.True(t, ok)
	require.NotEmpty(t, items, "platform capability should be visible to tenants")
	first := items[0].(map[string]interface{})
	require.Equal(t, "com.corex.media.assets.manage", first["capability_id"])
	require.Equal(t, "corex", first["source"])

	// Invoke capability via /tenant/invocations.
	body := map[string]any{
		"capability_id": "com.corex.media.assets.manage",
		"payload": map[string]any{
			"name": "tenant-upload.png",
		},
	}
	data, err := json.Marshal(body)
	require.NoError(t, err)

	invokeReq := httptest.NewRequest(http.MethodPost, "/api/tenant/invocations", bytes.NewReader(data))
	invokeReq.Header.Set("Content-Type", "application/json")
	invokeReq = withTenantContext(invokeReq, tenantUUID)
	invokeResp := httptest.NewRecorder()
	engine.ServeHTTP(invokeResp, invokeReq)

	require.Equal(t, http.StatusOK, invokeResp.Code)

	var invokePayload map[string]interface{}
	require.NoError(t, json.Unmarshal(invokeResp.Body.Bytes(), &invokePayload))
	require.Equal(t, "trace-corex-media", invokePayload["trace_id"])
	require.Equal(t, "completed", invokePayload["status"])

	require.Equal(t, "com.corex.media.assets.manage", invoker.lastInput.CapabilityID)
	require.Equal(t, tenantUUID, invoker.lastInput.TenantUUID)
	require.Equal(t, "rest", invoker.lastInput.PreferredProtocol)
	require.Equal(t, "media.assets", invoker.lastInput.Context["tool_scope"])
}

func initCapabilityDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.CapabilityRecord{}, &models.WorkflowTemplateRef{}, &models.WorkflowTemplateApproval{}))
	return db
}

func insertPlatformMediaRecord(t *testing.T, repo *caprepo.CapabilityRecordRepository) {
	t.Helper()

	record := &models.CapabilityRecord{
		CapabilityID:  "com.corex.media.assets.manage",
		PluginID:      "corex.platform",
		PluginVersion: "2025.01",
		Title:         "Media Assets Management",
		Description:   "Expose create/delete/presign for tenant media library.",
		Intents:       datatypes.JSON([]byte(`["media.assets.write"]`)),
		ToolScope:     datatypes.JSON([]byte(`["media.assets"]`)),
		Policy:        datatypes.JSON([]byte(`{"prefer":"rest"}`)),
		Protocols: datatypes.JSON([]byte(`[
			{"channel":"rest","endpoint":"/api/v1/media/assets","method":"POST","auth_type":"tenant_jwt","tool_scope":"media.assets"}
		]`)),
		CapabilitiesHash: "corex-media-hash",
		ProtocolHash:     "corex-media-protocol",
		Status:           "published",
	}
	_, err := repo.Upsert(context.Background(), record)
	require.NoError(t, err)
}

type stubCapabilityInvoker struct {
	lastInput capservice.InvocationInput
}

func (s *stubCapabilityInvoker) Invoke(ctx context.Context, in capservice.InvocationInput) (capservice.InvocationResult, error) {
	s.lastInput = in
	return capservice.InvocationResult{
		TraceID:      "trace-corex-media",
		Status:       "completed",
		ProtocolUsed: "rest",
		Result:       map[string]interface{}{"ok": true},
	}, nil
}

func withTenantContext(req *http.Request, tenantUUID string) *http.Request {
	ctx := reqctx.WithTenantUUID(req.Context(), tenantUUID)
	ctx = reqctx.WithTraceID(ctx, "trace-"+tenantUUID)
	return req.WithContext(ctx)
}
