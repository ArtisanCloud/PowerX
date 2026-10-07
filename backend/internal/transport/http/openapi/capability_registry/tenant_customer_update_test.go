package capability_registry

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	customersvc "github.com/ArtisanCloud/PowerX/internal/service/customer"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCustomerUpdateErrorsPreserveHTTPContract(t *testing.T) {
	for _, tc := range []struct {
		err          error
		status       int
		code, reason string
	}{
		{customersvc.ErrCustomerAccountInvalidArgument, 400, "registry.invalid_request", "CUSTOMER_ACCOUNT_INVALID_ARGUMENT"},
		{customersvc.ErrCustomerAccountNotFound, 404, "registry.not_found", "CUSTOMER_ACCOUNT_NOT_FOUND"},
		{customersvc.ErrExternalIdentityConflict, 409, "registry.invalid_request", "CUSTOMER_EXTERNAL_IDENTITY_CONFLICT"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Request = httptest.NewRequest("POST", "/api/v1/tenant/invocations", nil)
			wrapped := fmt.Errorf("update: %w", tc.err)
			selectInvokeErrorTemplate(wrapped).Respond(ctx, wrapped)
			require.Equal(t, tc.status, response.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			details := body["details"].(map[string]any)
			require.Equal(t, tc.code, details["code"])
			require.Equal(t, tc.reason, details["reason_code"])
		})
	}
}
