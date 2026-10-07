package customer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestListContactsRejectsTenantQueryWithoutTrustedContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/admin/customers/11111111-1111-1111-1111-111111111111/contacts?tenant_uuid=22222222-2222-2222-2222-222222222222", nil)
	context.Params = gin.Params{{Key: "customer_uuid", Value: "11111111-1111-1111-1111-111111111111"}}

	(&Handler{}).ListContacts(context)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestContactOpenAPIContractDeclaresEveryRegisteredContactRouteWithoutTenantOverride(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	contractPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../../../../specs/030-customer-contact/contracts/http-openapi.yaml"))
	raw, err := os.ReadFile(contractPath)
	require.NoError(t, err)
	var document struct {
		OpenAPI string                 `yaml:"openapi"`
		Paths   map[string]interface{} `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &document))
	require.Equal(t, "3.0.3", document.OpenAPI)
	for _, path := range []string{
		"/admin/customers/{customer_uuid}/contacts",
		"/admin/customers/{customer_uuid}/contacts/{contact_uuid}",
		"/admin/customers/{customer_uuid}/contacts:resolve-identity",
		"/admin/customers/{customer_uuid}/contacts/{contact_uuid}/identities",
	} {
		require.Contains(t, document.Paths, path)
	}
	require.NotContains(t, string(raw), "name: tenant_uuid")
}
