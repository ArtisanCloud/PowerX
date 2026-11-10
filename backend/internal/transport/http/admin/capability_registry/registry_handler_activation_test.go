package capability_registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/service/capability_registry/registry"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type activationHandlerRepository struct {
	registry.Repository
	current registry.Registration
}

func (r *activationHandlerRepository) GetLatest(context.Context, *gorm.DB, string, string) (registry.Registration, error) {
	if r.current.Version == 0 {
		return registry.Registration{}, registry.ErrRegistrationNotFound
	}
	return r.current, nil
}

func (r *activationHandlerRepository) Create(_ context.Context, _ *gorm.DB, value registry.Registration) (registry.Registration, error) {
	r.current = value
	return value, nil
}

func (r *activationHandlerRepository) Update(_ context.Context, _ *gorm.DB, value registry.Registration, expected uint64) (registry.Registration, error) {
	if r.current.Version != expected {
		return registry.Registration{}, registry.ErrVersionConflict
	}
	r.current = value
	return value, nil
}

func TestRegistrationHTTPPreservesAndReportsAdapterActivation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &activationHandlerRepository{}
	handler := NewAdminHandler(AdminHandlerOptions{Service: registry.NewService(registry.ServiceOptions{Repository: repo})})
	engine := gin.New()
	engine.POST("/admin/capability-registry/capabilities", handler.CreateCapability)
	engine.PUT("/admin/capability-registry/capabilities/:capabilityId/tenants/:tenant_uuid", handler.UpdateCapability)
	engine.GET("/admin/capability-registry/capabilities/:capabilityId/tenants/:tenant_uuid", handler.GetCapability)
	tenant := "6b5d0240-9920-46da-b707-88200e0f51ea"
	path := "/admin/capability-registry/capabilities/com.powerx.plugins.ecommerce.plan.read/tenants/" + tenant
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		engine.ServeHTTP(out, req)
		return out
	}
	for _, tc := range []struct {
		name, method, activation string
		want                     bool
	}{
		{"create defaults active", "POST", "", true},
		{"explicit disable", "PUT", `,"is_active":false`, false},
		{"omitted update preserves disabled", "PUT", "", false},
		{"explicit reenable", "PUT", `,"is_active":true`, true},
		{"omitted update preserves active", "PUT", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"capability_id":"com.powerx.plugins.ecommerce.plan.read","tenant_uuid":%q,"contract_ref":"2.0.0","status":"published","version":%d,"adapters":[{"adapter_id":"plans.POST","transport_type":"rest","endpoint_url":"/_p/com.powerx.plugins.ecommerce/api/v1/plugin/commerce/subscription-plans/query","weight":100,"timeout_ms":30000%s}],"routing_policy":{"strategy":"weighted_round_robin","cooldown_seconds":60}}`, tenant, repo.current.Version, tc.activation)
			// Create requests omit the optimistic update version.
			if tc.method == "POST" {
				body = strings.Replace(body, `,"version":0`, "", 1)
			}
			endpoint := path
			if tc.method == "POST" {
				endpoint = "/admin/capability-registry/capabilities"
			}
			out := request(tc.method, endpoint, body)
			require.Contains(t, []int{200, 201}, out.Code, out.Body.String())
			require.Equal(t, tc.want, repo.current.Adapters[0].IsActive)
			out = request("GET", path, "")
			require.Equal(t, 200, out.Code)
			var response struct {
				Data struct {
					Adapters []struct {
						IsActive *bool `json:"is_active"`
					} `json:"adapters"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(out.Body.Bytes(), &response))
			require.Len(t, response.Data.Adapters, 1)
			require.NotNil(t, response.Data.Adapters[0].IsActive)
			require.Equal(t, tc.want, *response.Data.Adapters[0].IsActive)
		})
	}
}
