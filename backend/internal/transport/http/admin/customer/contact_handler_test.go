package customer

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
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
