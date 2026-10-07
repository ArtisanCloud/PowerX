package knowledge_space

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	svc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	gwmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRegisteredHostProvisioningRoutesRejectMissingGrantsAndRawBodies(t *testing.T) {
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&capmodels.CapabilityRecord{}, &capmodels.CapabilityRegistration{}, &gwmodels.IntegrationGatewayAPIKey{}, &gwmodels.IntegrationGatewayAPIKeyPermission{}))
	tenant := uuid.NewString()
	for _, id := range []string{svc.KnowledgeCatalogReadCapabilityID, svc.KnowledgeSpaceCreateCapabilityID} {
		require.NoError(t, db.Create(&capmodels.CapabilityRecord{CapabilityID: id, PluginID: "com.powerx.core", PluginVersion: "v1", Title: "Knowledge", Status: "published"}).Error)
		require.NoError(t, db.Create(&capmodels.CapabilityRegistration{CapabilityID: id, TenantUUID: tenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	}
	key := gwmodels.IntegrationGatewayAPIKey{TenantUUID: tenant, ProfileID: 1, Name: "Host", KeyPrefix: "pxk", KeyHash: "host-route", Status: "active"}
	require.NoError(t, db.Create(&key).Error)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(c.Request.Context(), tenant)
		ctx = reqctx.WithClaims(ctx, &reqctx.CoreXClaims{TenantUUID: tenant, Platforms: []string{"api_key"}})
		c.Request = c.Request.WithContext(ctx)
		c.Set("auth_api_key_hash", key.KeyHash)
		c.Next()
	})
	deps := &shared.Deps{DB: db, KnowledgeSpace: &shared.KnowledgeSpaceDeps{Service: svc.NewService(svc.ServiceOptions{DB: db})}}
	Register(nil, engine.Group("/api/v1"), deps)
	call := func(method, path, body string) int {
		request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response.Code
	}
	require.Equal(t, http.StatusForbidden, call("GET", "/api/v1/tenant/knowledge/catalog", ""))
	require.Equal(t, http.StatusForbidden, call("POST", "/api/v1/tenant/knowledge/spaces", "{}"))
	require.NoError(t, db.Create(&gwmodels.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: "_scope.knowledge.space.create", Action: "create", ResourceType: "api", ResourcePattern: "space", Effect: "allow"}).Error)
	for _, body := range []string{
		`{"name":"Bad","department_uuid":"bad","strategy_key":"A_simple","tenant_uuid":"override"}`,
		`{"name":"Bad","department_uuid":"bad","strategy_key":"A_simple","headers":{}}`,
		`{} {}`,
	} {
		require.Equal(t, http.StatusBadRequest, call("POST", "/api/v1/tenant/knowledge/spaces", body))
	}
	require.Equal(t, http.StatusBadRequest, call("POST", "/api/v1/tenant/knowledge/spaces?tenant_uuid=override", "{}"))
}
