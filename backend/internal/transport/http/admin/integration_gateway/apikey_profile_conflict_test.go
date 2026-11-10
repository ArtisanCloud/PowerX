package integration_gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	owners "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gwmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	gwrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAPIKeyProfileHTTPConflictDetailsAndRollback(t *testing.T) {
	previous := model.PowerXSchema
	model.PowerXSchema = "main"
	t.Cleanup(func() { model.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&iammodel.APIKeyProfile{}, &iammodel.APIKeyProfilePermission{}, &iammodel.Permission{}, &gwmodel.IntegrationGatewayAPIKey{}, &gwmodel.IntegrationGatewayAPIKeyPermission{}, &gwmodel.IntegrationGatewayAPIKeyAuditLog{}))
	tenant := uuid.NewString()
	profile := iammodel.APIKeyProfile{TenantUUID: tenant, Name: "Shared fixture", Key: "shared", Status: 1}
	require.NoError(t, db.Create(&profile).Error)
	makePermission := func(resource, plugin string) iammodel.Permission {
		t.Helper()
		meta, _ := json.Marshal(map[string]any{"label": "Permission " + resource, "capability_id": "com.powerx.plugins.ecommerce." + resource + ".read", "api_key": map[string]any{"scope": "_scope." + resource + ".read", "action": "read", "resource_type": "capability", "resource_pattern": resource, "plugin_id": plugin, "effect": "allow"}})
		p := iammodel.Permission{Module: "fixture", Resource: resource, Action: "read", AllowAPIKey: true, Status: iammodel.PermissionStatusActive, Meta: datatypes.JSON(meta)}
		require.NoError(t, db.Create(&p).Error)
		return p
	}
	core := makePermission("jobs", "")
	fixed := makePermission("plans", "com.powerx.plugins.ecommerce")
	require.NoError(t, iamrepo.NewAPIKeyProfilePermissionRepository(db).GrantByIDsTx(db, profile.ID, []uint64{core.ID}))
	svc := owners.NewAPIKeyOwnerService(db)
	keys := []gwmodel.IntegrationGatewayAPIKey{}
	for _, name := range []string{"workspace-key", "crm-key"} {
		key := gwmodel.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: profile.ID, Name: name, Status: "active", KeyPrefix: "must-not-return-prefix", KeyHash: uuid.NewString(), PluginOwnerPolicy: owners.EncodePluginOwners([]string{"com.example." + name})}
		require.NoError(t, db.Create(&key).Error)
		keys = append(keys, key)
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, _, err := svc.SyncProfileTx(context.Background(), tx, tenant, profile.ID)
		return err
	}))
	handler := NewAPIKeyAdminHandler(db)
	handler.templatesEnsuredOnce = true
	router := gin.New()
	router.Use(func(c *gin.Context) {
		current := tenant
		if c.GetHeader("X-Test-Tenant") != "" {
			current = c.GetHeader("X-Test-Tenant")
		}
		ctx := reqctx.WithTenantUUID(reqctx.WithClaims(c.Request.Context(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, TenantUUID: current}), current)
		c.Request = c.Request.WithContext(reqctx.WithTraceID(ctx, "owner-conflict-http-trace"))
		c.Next()
	})
	router.PUT("/profiles/:profile_id/permissions", handler.SetAPIKeyProfilePermissions)
	router.POST("/profiles/:profile_id/permissions/append", handler.AppendAPIKeyProfilePermissions)
	router.PUT("/keys/:key_id/plugin-owners", NewAPIKeyOwnerHandler(svc).SetPluginOwners)
	path := "/profiles/" + strconv.FormatUint(profile.ID, 10) + "/permissions"
	call := func(method, path string, body any, foreign bool, status int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		request := httptest.NewRequest(method, path, bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		if foreign {
			request.Header.Set("X-Test-Tenant", uuid.NewString())
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, status, response.Code, response.Body.String())
		var envelope map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
		return envelope
	}
	for _, operation := range []struct{ method, suffix string }{{"PUT", ""}, {"POST", "/append"}} {
		body := call(operation.method, path+operation.suffix, map[string]any{"permission_ids": []uint64{core.ID, fixed.ID}}, false, 400)
		require.Equal(t, "API_KEY_PLUGIN_OWNER_CONFLICT", body["error_code"])
		require.Equal(t, "API_KEY_PLUGIN_OWNER_CONFLICT", body["reason_code"])
		details := body["details"].(map[string]any)
		require.EqualValues(t, 2, details["conflict_count"])
		require.EqualValues(t, 2, details["conflicting_key_count"])
		require.Equal(t, profile.Name, details["profile_name"])
		require.Equal(t, "owner-conflict-http-trace", details["trace_id"])
		conflicts := details["conflicts"].([]any)
		require.Len(t, conflicts, 2)
		for _, value := range conflicts {
			item := value.(map[string]any)
			require.EqualValues(t, fixed.ID, item["permission_id"])
			require.Equal(t, "Permission plans", item["permission_name"])
			require.Equal(t, "com.powerx.plugins.ecommerce", item["required_plugin_id"])
			require.NotEmpty(t, item["key_name"])
		}
		serialized, _ := json.Marshal(body)
		require.NotContains(t, string(serialized), "must-not-return-prefix")
		for _, key := range keys {
			require.NotContains(t, string(serialized), key.KeyHash)
			stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(context.Background(), key.UUID)
			require.NoError(t, err)
			require.Equal(t, key.PluginOwnerPolicy, stored.PluginOwnerPolicy)
			rows, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ListByAPIKeyUUID(context.Background(), key.UUID)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, "jobs", rows[0].ResourcePattern)
		}
		ids, err := iamrepo.NewAPIKeyProfilePermissionRepository(db).ListPermissionIDsOfProfile(context.Background(), profile.ID)
		require.NoError(t, err)
		require.Equal(t, []uint64{core.ID}, ids)
	}
	foreign := call("PUT", path, map[string]any{"permission_ids": []uint64{fixed.ID}}, true, 404)
	require.NotContains(t, foreign, "details")
	// 清空 owner 的 HTTP 入口同样反馈具体权限，并保留原 owner。
	require.NoError(t, iamrepo.NewAPIKeyProfilePermissionRepository(db).GrantByIDsTx(db, profile.ID, []uint64{fixed.ID}))
	target := keys[0]
	original := owners.EncodePluginOwners([]string{"com.powerx.plugins.ecommerce"})
	require.NoError(t, db.Model(&target).Update("plugin_owner_policy", original).Error)
	body := call("PUT", "/keys/"+target.UUID.String()+"/plugin-owners", map[string]any{"plugin_ids": []string{}}, false, 400)
	require.Equal(t, "API_KEY_PLUGIN_OWNER_CONFLICT", body["error_code"])
	details := body["details"].(map[string]any)
	conflicts := details["conflicts"].([]any)
	require.Len(t, conflicts, 1)
	require.Equal(t, target.UUID.String(), conflicts[0].(map[string]any)["key_id"])
	require.Equal(t, []any{}, conflicts[0].(map[string]any)["owner_plugin_ids"])
	stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(context.Background(), target.UUID)
	require.NoError(t, err)
	require.Equal(t, original, stored.PluginOwnerPolicy)
}
