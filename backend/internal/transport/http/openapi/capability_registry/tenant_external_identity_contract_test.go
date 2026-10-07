package capability_registry

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const externalIdentityContractTenantUUID = "11111111-1111-1111-1111-111111111111"

func TestTenantInvocationsExternalIdentityContract(t *testing.T) {
	tests := []struct {
		name       string
		withTenant bool
		err        error
		wantStatus int
		wantReason string
	}{
		{"missing tenant", false, nil, http.StatusUnauthorized, ""},
		{"invalid service actor", true, customerrepo.ErrExternalIdentityServiceActorInvalid, http.StatusUnauthorized, "CUSTOMER_EXTERNAL_IDENTITY_SERVICE_ACTOR_INVALID"},
		{"capability grant missing", true, capservice.ErrSelectorCapabilityForbidden, http.StatusForbidden, "CAPABILITY_FORBIDDEN"},
		{"invalid payload", true, customerrepo.ErrExternalIdentityRequired, http.StatusBadRequest, "CUSTOMER_EXTERNAL_IDENTITY_INVALID"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			invoker := &externalIdentityContractInvoker{err: tt.err}
			engine := externalIdentityContractEngine(invoker, tt.withTenant)
			response := invokeExternalIdentityContract(t, engine, map[string]interface{}{
				"capability_id":      "com.corex.customer.external_identities.resolve",
				"preferred_protocol": "core_internal",
				"payload": map[string]interface{}{
					"method":   "INVOKE",
					"endpoint": "core://customer/external-identities/resolve",
					"body": map[string]interface{}{
						"provider_subject": "shop:immutable:customer:1",
						"display_name":     "客户甲",
					},
				},
			})
			require.Equal(t, tt.wantStatus, response.Code)
			if tt.wantReason == "" {
				return
			}
			var envelope map[string]interface{}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
			details := envelope["details"].(map[string]interface{})
			require.Equal(t, tt.wantReason, details["reason_code"])
		})
	}
}

func TestTenantInvocationsExternalIdentityDerivesTenantFromCredential(t *testing.T) {
	invoker := &externalIdentityContractInvoker{result: capservice.InvocationResult{TraceID: "trace-1", Status: "completed", ProtocolUsed: "core_internal", Result: map[string]interface{}{"item": map[string]interface{}{"customer_uuid": "33333333-3333-3333-3333-333333333333"}}}}
	engine := externalIdentityContractEngine(invoker, true)
	response := invokeExternalIdentityContract(t, engine, map[string]interface{}{
		"capability_id": "com.corex.customer.external_identities.resolve",
		"payload": map[string]interface{}{
			"method": "INVOKE", "endpoint": "core://customer/external-identities/resolve",
			"body": map[string]interface{}{"provider_subject": "shop:immutable:customer:1", "display_name": "客户甲"},
		},
	})
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, externalIdentityContractTenantUUID, invoker.input.TenantUUID)
}

func externalIdentityContractEngine(invoker *externalIdentityContractInvoker, withTenant bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	selector := capservice.NewSelector(capservice.SelectorOptions{Invoker: invoker})
	deps := &shared.Deps{CapabilityCatalogSvc: &capservice.RegistryService{}, CapabilitySelector: selector}
	engine := gin.New()
	if withTenant {
		engine.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(reqctx.WithTenantUUID(c.Request.Context(), externalIdentityContractTenantUUID))
			c.Next()
		})
	}
	RegisterTenantRoutes(engine.Group("/api/v1"), deps)
	return engine
}

func invokeExternalIdentityContract(t *testing.T, engine *gin.Engine, body map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenant/invocations", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

type externalIdentityContractInvoker struct {
	input  capservice.InvocationInput
	result capservice.InvocationResult
	err    error
}

func (i *externalIdentityContractInvoker) Invoke(_ context.Context, input capservice.InvocationInput) (capservice.InvocationResult, error) {
	i.input = input
	if i.err != nil {
		return capservice.InvocationResult{}, i.err
	}
	if i.result.Status == "" {
		i.result = capservice.InvocationResult{TraceID: "trace-default", Status: "completed", ProtocolUsed: "core_internal"}
	}
	return i.result, nil
}
