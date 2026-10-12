package ai

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	aisvc "github.com/ArtisanCloud/PowerX/internal/service/ai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRespondAIErrorRejectsInvalidSamplingParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/ai/llm/invoke", nil)
	respondAIError(ctx, fmt.Errorf("%w: seed must be an integer", aisvc.ErrInvalidLLMParams))
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}
