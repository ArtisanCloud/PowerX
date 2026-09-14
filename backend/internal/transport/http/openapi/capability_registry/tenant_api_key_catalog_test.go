package capability_registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	iammodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/iam"
	gwmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const apiKeyCatalogTestTenant = "22222222-2222-2222-2222-222222222222"

func TestTenantInvocationsAPIKeyCatalogReads(t *testing.T) {
	previousSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previousSchema })

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&capmodel.CapabilityRecord{}, &capmodel.CapabilityRegistration{},
		&iammodel.Permission{}, &gwmodel.IntegrationGatewayAPIKey{}, &gwmodel.IntegrationGatewayAPIKeyPermission{},
	))

	const keyHash = "catalog-api-key-hash"
	key := gwmodel.IntegrationGatewayAPIKey{TenantUUID: apiKeyCatalogTestTenant, ProfileID: 1, Name: "catalog", KeyPrefix: "test", KeyHash: keyHash, Status: "active"}
	require.NoError(t, db.Create(&key).Error)

	capabilities := map[string]struct {
		scope    string
		endpoint string
	}{
		"com.corex.ai.catalog.providers.read": {scope: "_scope.ai.catalog.providers.read", endpoint: "/api/v1/admin/agents/providers"},
		"com.corex.ai.catalog.models.read":    {scope: "_scope.ai.catalog.models.read", endpoint: "/api/v1/admin/agents/models"},
	}
	for capabilityID, spec := range capabilities {
		seedAPIKeyCatalogCapability(t, db, key, capabilityID, spec.scope)
	}

	invoker := &apiKeyCatalogInvoker{}
	selector := capservice.NewSelector(capservice.SelectorOptions{Invoker: invoker})
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		ctx := reqctx.WithAuthenticatedAPIKeyHash(reqctx.WithClaims(reqctx.WithTenantUUID(c.Request.Context(), apiKeyCatalogTestTenant), &reqctx.CoreXClaims{
			TenantUUID: apiKeyCatalogTestTenant,
			Platforms:  []string{"api_key"},
		}), keyHash)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	RegisterTenantRoutes(engine.Group("/api/v1"), &shared.Deps{
		DB:                   db,
		CapabilityCatalogSvc: capservice.NewRegistryService(capservice.RegistryServiceOptions{DB: db}),
		CapabilitySelector:   selector,
	})

	for capabilityID, spec := range capabilities {
		t.Run(capabilityID, func(t *testing.T) {
			body, marshalErr := json.Marshal(map[string]interface{}{
				"capability_id": capabilityID,
				"payload": map[string]interface{}{
					"method":   http.MethodGet,
					"endpoint": spec.endpoint,
					"query":    map[string]interface{}{"modality": "llm"},
				},
			})
			require.NoError(t, marshalErr)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/tenant/invocations", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.Equal(t, capabilityID, invoker.input.CapabilityID)
			require.Equal(t, "llm", invoker.input.Payload["query"].(map[string]interface{})["modality"])
		})
	}
}

func seedAPIKeyCatalogCapability(t *testing.T, db *gorm.DB, key gwmodel.IntegrationGatewayAPIKey, capabilityID, scope string) {
	t.Helper()
	require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: capabilityID, PluginID: "core", PluginVersion: "test", Title: capabilityID, CapabilitiesHash: capabilityID, ProtocolHash: capabilityID, Status: "published"}).Error)
	require.NoError(t, db.Create(&capmodel.CapabilityRegistration{CapabilityID: capabilityID, TenantUUID: apiKeyCatalogTestTenant, ContractRef: "test", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
	meta, err := json.Marshal(map[string]interface{}{"capability_id": capabilityID, "api_key_explicit": true, "api_key": map[string]string{"scope": scope, "action": "read", "resource_type": "api", "resource_pattern": capabilityID, "effect": "allow"}})
	require.NoError(t, err)
	permission := iammodel.Permission{Module: "ai", Resource: capabilityID, Action: "read", Effect: "allow", Status: iammodel.PermissionStatusActive, AllowAPIKey: true, Meta: datatypes.JSON(meta)}
	require.NoError(t, db.Create(&permission).Error)
	require.NoError(t, db.Create(&gwmodel.IntegrationGatewayAPIKeyPermission{APIKeyUUID: key.UUID, Scope: scope, Action: "read", ResourceType: "api", ResourcePattern: capabilityID, Effect: "allow"}).Error)
}

type apiKeyCatalogInvoker struct{ input capservice.InvocationInput }

func (i *apiKeyCatalogInvoker) Invoke(_ context.Context, input capservice.InvocationInput) (capservice.InvocationResult, error) {
	i.input = input
	return capservice.InvocationResult{TraceID: "catalog-trace", Status: "completed", ProtocolUsed: "rest", Result: map[string]interface{}{"items": []interface{}{}}}, nil
}
