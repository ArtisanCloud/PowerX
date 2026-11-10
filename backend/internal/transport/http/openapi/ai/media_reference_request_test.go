package ai

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	aisvc "github.com/ArtisanCloud/PowerX/internal/service/ai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMediaRESTReferenceEnvelopeLimitAndFailureCode(t *testing.T) {
	h := &aiHandler{media: &aisvc.MediaRuntime{}}
	for _, tc := range []struct {
		size, status int
		code         string
	}{
		{14 << 20, 503, "AI_TASKBUS_UNAVAILABLE"}, // 大于旧限制，成功进入任务服务校验。
		{contract.MediaRequestMaxBytes, 422, "AI_INPUT_LIMIT_EXCEEDED"},
	} {
		body := `{"operation":"invoke","contract_version":2,"model_key":"openai/gpt-image-2","idempotency_key":"fixture","inputs":[{"type":"image_url","url":"data:image/png;base64,` + strings.Repeat("A", tc.size) + `"}]}`
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest("POST", "/api/v1/ai/image/invoke", strings.NewReader(body))
		h.invokeMedia(ctx, "image_generation")
		require.Equal(t, tc.status, response.Code)
		require.Contains(t, response.Body.String(), tc.code)
	}
}
