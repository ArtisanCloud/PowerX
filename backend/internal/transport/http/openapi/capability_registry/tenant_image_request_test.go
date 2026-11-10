package capability_registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestImageInvocationAllowsReferenceEnvelopeWithoutExpandingOtherCapabilities(t *testing.T) {
	for _, tc := range []struct {
		id, protocol string
		allowed      bool
	}{
		{"com.corex.ai.image.invoke", "core_internal", true},
		{"com.corex.ai.image.invoke", "rest", false},
		{"com.corex.ai.video.invoke", "core_internal", false},
		{"com.corex.ai.catalog.models.read", "core_internal", false},
	} {
		body, err := json.Marshal(map[string]any{"capability_id": tc.id, "preferred_protocol": tc.protocol, "payload": map[string]any{"body": map[string]any{"inputs": []any{map[string]any{"type": "image_url", "url": "data:image/png;base64," + strings.Repeat("A", 9<<20)}}}}})
		require.NoError(t, err)
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/tenant/invocations", strings.NewReader(string(body)))
		var in capabilityInvokeRequest
		err = decodeInvocationRequest(ctx, &in)
		if tc.allowed {
			require.NoError(t, err)
			require.Equal(t, tc.id, in.CapabilityID)
		} else {
			require.Error(t, err)
		}
	}
}

func TestImageInvocationRejectsOversizeAndStrictSchema(t *testing.T) {
	body := `{"capability_id":"com.corex.ai.image.invoke","preferred_protocol":"core_internal","payload":{"body":"` + strings.Repeat("A", contract.MediaRequestMaxBytes) + `"}}`
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/api/v1/tenant/invocations", strings.NewReader(body))
	var in capabilityInvokeRequest
	err := decodeInvocationRequest(ctx, &in)
	var failure *dto.AppError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, "AI_INPUT_LIMIT_EXCEEDED", failure.Code)
	response := httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest("POST", "/api/v1/tenant/invocations", nil)
	respondInvocationDecodeError(ctx, err)
	require.Equal(t, 422, response.Code)
	require.Contains(t, response.Body.String(), "AI_INPUT_LIMIT_EXCEEDED")
	for _, body := range []string{
		`{"capability_id":"com.corex.ai.image.invoke","endpoint":"https://example.invalid"}`,
		`{"capability_id":"com.corex.ai.image.invoke"}{}`,
	} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("POST", "/api/v1/tenant/invocations", strings.NewReader(body))
		require.Error(t, decodeInvocationRequest(ctx, &in))
	}
}
