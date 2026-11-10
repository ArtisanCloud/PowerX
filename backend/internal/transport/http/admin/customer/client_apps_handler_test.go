package customer

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClientAppsRejectTenantOverrideAndMalformedJSON(t *testing.T) {
	for _, body := range []string{`{"tenant_uuid":"override"}`, `{"connections":[{"provider":"wechat","app_id":"wx","tenant_uuid":"override"}]}`, `{} {}`, strings.Repeat(" ", 33<<10) + `{}`} {
		router := gin.New()
		h := &Handler{}
		router.POST("/apps", h.SaveClientApp)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("POST", "/apps", strings.NewReader(body)))
		if len(body) > 32<<10 {
			require.Equal(t, 413, response.Code)
			require.Contains(t, response.Body.String(), "CLIENT_APP_REQUEST_TOO_LARGE")
		} else {
			require.Equal(t, 400, response.Code)
		}
		if len(body) <= 32<<10 {
			require.Contains(t, response.Body.String(), "CLIENT_APP_INVALID_ARGUMENT")
		}
	}
	router := gin.New()
	h := &Handler{}
	router.POST("/apps", h.SaveClientApp)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("POST", "/apps?tenant_uuid=override", strings.NewReader(`{}`)))
	require.Equal(t, 400, response.Code)
}

func TestClientApplicationsOpenAPI(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../../docs/contracts/client-applications.openapi.yaml"))
	contract, err := openapi3.NewLoader().LoadFromFile(path)
	require.NoError(t, err)
	require.NoError(t, contract.Validate(context.Background()))
	require.Len(t, contract.Paths, 4)
}
