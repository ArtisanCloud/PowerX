package capability_registry

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestKnowledgeTypedInvocationPreservesHTTPStatusAndReason(t *testing.T) {
	for _, status := range []int{400, 403, 404, 409, 422, 501, 503} {
		reason := fmt.Sprintf("KNOWLEDGE_TEST_%d", status)
		err := fmt.Errorf("invoke: %w", dto.NewErrorWithCode(status, reason, reason, errors.New("test")))
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest("POST", "/api/v1/tenant/invocations", nil)
		selectInvokeErrorTemplate(err).Respond(ctx, err)
		require.Equal(t, status, response.Code)
		require.Contains(t, response.Body.String(), reason)
	}
}
