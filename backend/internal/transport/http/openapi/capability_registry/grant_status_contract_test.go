package capability_registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	capability_registrydto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/capability_registry/dto"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

func TestCapabilityGrantStatusContract(t *testing.T) {
	tests := []struct {
		name       string
		claims     *reqctx.CoreXClaims
		body       map[string]any
		serviceErr error
		wantStatus int
		wantReason string
		wantIDs    []string
	}{
		{
			name:       "sts success",
			claims:     &reqctx.CoreXClaims{PluginID: "com.powerx.plugins.scrm", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}}},
			body:       map[string]any{"capability_ids": []string{"com.powerx.plugins.scrm.leads.write", "com.powerx.plugins.scrm.leads.read"}},
			wantStatus: http.StatusOK, wantIDs: []string{"com.powerx.plugins.scrm.leads.write", "com.powerx.plugins.scrm.leads.read"},
		},
		{
			name:       "api key success",
			claims:     &reqctx.CoreXClaims{MemberID: 42, Platforms: []string{"api_key"}},
			body:       map[string]any{"capability_ids": []string{"com.powerx.plugins.scrm.leads.read"}},
			wantStatus: http.StatusOK, wantIDs: []string{"com.powerx.plugins.scrm.leads.read"},
		},
		{
			name:       "tenant override rejected",
			claims:     &reqctx.CoreXClaims{PluginID: "com.powerx.plugins.scrm", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}}},
			body:       map[string]any{"tenant_uuid": "22222222-2222-2222-2222-222222222222", "capability_ids": []string{"com.powerx.plugins.scrm.leads.read"}},
			wantStatus: http.StatusBadRequest, wantReason: "CAPABILITY_GRANT_STATUS_INVALID_ARGUMENT",
		},
		{
			name:       "plugin override rejected",
			claims:     &reqctx.CoreXClaims{PluginID: "com.powerx.plugins.scrm", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}}},
			body:       map[string]any{"plugin_id": "other", "capability_ids": []string{"com.powerx.plugins.scrm.leads.read"}},
			wantStatus: http.StatusBadRequest, wantReason: "CAPABILITY_GRANT_STATUS_INVALID_ARGUMENT",
		},
		{
			name:       "non service jwt forbidden",
			claims:     &reqctx.CoreXClaims{UserID: 7},
			body:       map[string]any{"capability_ids": []string{"com.powerx.plugins.scrm.leads.read"}},
			wantStatus: http.StatusForbidden, wantReason: "CAPABILITY_GRANT_STATUS_FORBIDDEN",
		},
		{
			name:   "service unavailable",
			claims: &reqctx.CoreXClaims{PluginID: "com.powerx.plugins.scrm", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}}},
			body:   map[string]any{"capability_ids": []string{"com.powerx.plugins.scrm.leads.read"}}, serviceErr: capservice.ErrGrantStatusUnavailable,
			wantStatus: http.StatusServiceUnavailable, wantReason: "CAPABILITY_GRANT_STATUS_UPSTREAM_DEPENDENCY",
		},
		{
			name:       "credential rejected",
			claims:     &reqctx.CoreXClaims{PluginID: "com.powerx.plugins.scrm", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}}},
			body:       map[string]any{"capability_ids": []string{"com.powerx.plugins.scrm.leads.read"}},
			serviceErr: capservice.ErrGrantStatusUnauthorized,
			wantStatus: http.StatusUnauthorized, wantReason: "CAPABILITY_GRANT_STATUS_UNAUTHORIZED",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &grantStatusContractReader{err: tt.serviceErr, items: []capservice.GrantStatusItem{
				{CapabilityID: "com.powerx.plugins.scrm.leads.write", Status: capservice.GrantStatusNotGranted, ReasonCode: capservice.GrantStatusReasonNotGranted},
				{CapabilityID: "com.powerx.plugins.scrm.leads.read", Status: capservice.GrantStatusGranted, ReasonCode: capservice.GrantStatusReasonGranted},
			}}
			engine := grantStatusContractEngine(reader, tt.claims)
			response := invokeGrantStatusContract(t, engine, tt.body)
			require.Equal(t, tt.wantStatus, response.Code)
			if tt.wantReason != "" {
				var envelope map[string]any
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
				details := envelope["details"].(map[string]any)
				require.Equal(t, tt.wantReason, details["reason_code"])
			}
			if tt.wantStatus == http.StatusOK {
				require.Equal(t, tt.wantIDs, reader.input)
				var envelope struct {
					Data struct {
						Items []capability_registrydto.GrantStatusItem `json:"items"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
				require.Len(t, envelope.Data.Items, 2)
				require.Equal(t, capservice.GrantStatusReasonNotGranted, envelope.Data.Items[0].ReasonCode)
				require.Equal(t, capservice.GrantStatusReasonGranted, envelope.Data.Items[1].ReasonCode)
			}
		})
	}
}

func TestCapabilityGrantStatusRejectsTenantAndPluginQueryOrHeaderOverrides(t *testing.T) {
	claims := &reqctx.CoreXClaims{PluginID: "com.powerx.plugins.scrm", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Audience: jwt.ClaimStrings{"powerx:api"}}}
	engine := grantStatusContractEngine(&grantStatusContractReader{items: []capservice.GrantStatusItem{}}, claims)
	for _, mutate := range []func(*http.Request){
		func(request *http.Request) { request.URL.RawQuery = "tenant_uuid=22222222-2222-2222-2222-222222222222" },
		func(request *http.Request) { request.URL.RawQuery = "plugin_id=other" },
		func(request *http.Request) {
			request.Header.Set("X-Tenant-UUID", "22222222-2222-2222-2222-222222222222")
		},
		func(request *http.Request) { request.Header.Set("X-Plugin-ID", "other") },
	} {
		raw := []byte(`{"capability_ids":["com.powerx.plugins.scrm.leads.read"]}`)
		request := httptest.NewRequest(http.MethodPost, "/api/v1/tenant/capabilities:grant-status", bytes.NewReader(raw))
		request.Header.Set("Content-Type", "application/json")
		mutate(request)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		require.Equal(t, http.StatusBadRequest, response.Code)
	}
}

func grantStatusContractEngine(reader grantStatusReader, claims *reqctx.CoreXClaims) *gin.Engine {
	gin.SetMode(gin.TestMode)
	deps := &shared.Deps{CapabilityCatalogSvc: &capservice.RegistryService{}}
	handler := newTenantHandler(deps)
	handler.grantStatus = reader
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(c.Request.Context(), externalIdentityContractTenantUUID)
		if claims != nil {
			ctx = reqctx.WithClaims(ctx, claims)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	engine.POST("/api/v1/tenant/capabilities:grant-status", handler.GetCapabilityGrantStatus)
	return engine
}

func invokeGrantStatusContract(t *testing.T, engine *gin.Engine, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/tenant/capabilities:grant-status", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

type grantStatusContractReader struct {
	input []string
	items []capservice.GrantStatusItem
	err   error
}

func (r *grantStatusContractReader) CheckCurrentCredential(_ context.Context, capabilityIDs []string) ([]capservice.GrantStatusItem, error) {
	r.input = append([]string(nil), capabilityIDs...)
	if r.err != nil {
		return nil, r.err
	}
	if r.items == nil {
		return nil, errors.New("missing test items")
	}
	return r.items, nil
}
