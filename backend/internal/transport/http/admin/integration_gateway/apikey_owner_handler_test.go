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

func TestAPIKeyOwnerHTTPCreateSaveAppendClearRestoreRotateAndIsolation(t *testing.T) {
	oldSchema := model.PowerXSchema
	model.PowerXSchema = "main"
	t.Cleanup(func() { model.PowerXSchema = oldSchema })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&iammodel.APIKey{}, &iammodel.APIKeyProfile{}, &iammodel.APIKeyProfilePermission{}, &iammodel.Permission{}, &gwmodel.IntegrationGatewayAPIKey{}, &gwmodel.IntegrationGatewayAPIKeyPermission{}, &gwmodel.IntegrationGatewayAPIKeyAuditLog{}))
	tenant := uuid.NewString()
	profile := &iammodel.APIKeyProfile{TenantUUID: tenant, Key: "owner-http", Name: "owner HTTP", Status: 1}
	require.NoError(t, db.Create(profile).Error)
	ids := []uint64{}
	for _, action := range []string{"read", "delete"} {
		meta, _ := json.Marshal(map[string]any{"api_key": map[string]any{"scope": "_scope.scheduler.jobs.service_" + action, "action": action, "resource_type": "api", "resource_pattern": "scheduler_jobs", "effect": "allow"}})
		p := &iammodel.Permission{Module: "scheduler", Resource: "jobs.service", Action: action, AllowAPIKey: true, Status: iammodel.PermissionStatusActive, Meta: datatypes.JSON(meta)}
		require.NoError(t, db.Create(p).Error)
		ids = append(ids, p.ID)
	}
	require.NoError(t, iamrepo.NewAPIKeyProfilePermissionRepository(db).GrantByIDsTx(db, profile.ID, ids))
	handler := NewAPIKeyAdminHandler(db)
	handler.templatesEnsuredOnce = true
	ownerHandler := NewAPIKeyOwnerHandler(owners.NewAPIKeyOwnerService(db))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		currentTenant := c.GetHeader("X-Test-Tenant")
		if currentTenant == "" {
			currentTenant = tenant
		}
		ctx := reqctx.WithTenantUUID(reqctx.WithClaims(context.Background(), &reqctx.CoreXClaims{UserID: 1, IsRoot: true, TenantUUID: currentTenant}), currentTenant)
		ctx = reqctx.WithTraceID(ctx, uuid.NewString())
		if c.GetHeader("X-Test-Auth") == "api_key" {
			ctx = reqctx.WithAuthenticatedAPIKeyHash(ctx, "authenticated-fixture")
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/keys", handler.CreateAPIKey)
	router.POST("/keys/:key_id/rotate", handler.RotateAPIKey)
	router.GET("/keys/:key_id/plugin-owners", ownerHandler.GetPluginOwners)
	router.PUT("/keys/:key_id/plugin-owners", ownerHandler.SetPluginOwners)
	router.PUT("/profiles/:profile_id/permissions", handler.SetAPIKeyProfilePermissions)
	router.POST("/profiles/:profile_id/permissions/append", handler.AppendAPIKeyProfilePermissions)
	request := func(method, path string, body any, headers map[string]string, status int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, status, w.Code, w.Body.String())
		var response map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
		if data, ok := response["data"].(map[string]any); ok {
			return data
		}
		return response
	}
	create := request("POST", "/keys", map[string]any{"profile_id": profile.ID, "name": "owner HTTP", "plugin_ids": []string{"com.powerx.plugins.scrm"}}, nil, 201)
	key := create["api_key"].(map[string]any)
	id := key["key_id"].(string)
	ownersPath := "/keys/" + id + "/plugin-owners"
	request("PUT", ownersPath, map[string]any{"plugin_ids": []string{"com.powerx.plugins.crm"}}, nil, 200)
	profilePath := "/profiles/" + strconv.FormatUint(profile.ID, 10) + "/permissions"
	request("PUT", profilePath, map[string]any{"permission_ids": ids}, nil, 200)
	request("POST", profilePath+"/append", map[string]any{"permission_ids": ids}, nil, 200)
	require.Equal(t, []any{"com.powerx.plugins.crm"}, request("GET", ownersPath, nil, nil, 200)["plugin_ids"])
	request("PUT", profilePath, map[string]any{"permission_ids": []uint64{}}, nil, 200)
	require.Equal(t, []any{"com.powerx.plugins.crm"}, request("GET", ownersPath, nil, nil, 200)["plugin_ids"])
	request("PUT", profilePath, map[string]any{"permission_ids": ids}, nil, 200)
	rotation := request("POST", "/keys/"+id+"/rotate", map[string]any{}, nil, 200)
	rotatedKey := rotation["api_key"].(map[string]any)
	require.Equal(t, []any{"com.powerx.plugins.crm"}, rotatedKey["plugin_ids"])
	rotatedID := rotatedKey["key_id"].(string)
	stored, err := gwrepo.NewIntegrationGatewayAPIKeyRepository(db).GetByUUID(context.Background(), uuid.MustParse(id))
	require.NoError(t, err)
	require.Equal(t, "revoked", stored.Status)
	rows, err := gwrepo.NewIntegrationGatewayAPIKeyPermissionRepository(db).ListByAPIKeyUUID(context.Background(), uuid.MustParse(rotatedID))
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.Equal(t, "com.powerx.plugins.crm", row.PluginID)
	}
	for _, body := range []any{map[string]any{"plugin_ids": []string{"*"}}, map[string]any{}, map[string]any{"plugin_ids": nil}, map[string]any{"plugin_ids": []string{"com.powerx.plugins.crm"}, "tenant_uuid": uuid.NewString()}} {
		request("PUT", "/keys/"+rotatedID+"/plugin-owners", body, nil, 400)
	}
	request("PUT", "/keys/"+rotatedID+"/plugin-owners", map[string]any{"plugin_ids": []string{"com.powerx.plugins.crm"}}, map[string]string{"X-Test-Tenant": uuid.NewString()}, 404)
	request("PUT", "/keys/"+rotatedID+"/plugin-owners", map[string]any{"plugin_ids": []string{"com.powerx.plugins.crm"}}, map[string]string{"X-Test-Auth": "api_key"}, 403)
}
