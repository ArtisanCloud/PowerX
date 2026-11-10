package ai

import (
	"context"
	aisvc "github.com/ArtisanCloud/PowerX/internal/service/ai"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionLLMStreamPreservesCallerBudgetInsteadOfLegacyHandlerTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, budget := range []time.Duration{10 * time.Minute, 30 * time.Second} {
		t.Run(budget.String(), func(t *testing.T) {
			tenant := "22222222-2222-2222-2222-222222222222"
			parent, cancel := context.WithTimeout(reqctx.WithEnv(reqctx.WithTenantUUID(context.Background(), tenant), "dev"), budget)
			defer cancel()
			expected, _ := parent.Deadline()
			called := false
			h := &aiHandler{aiInvokeTimeout: time.Millisecond, sessions: newSessionStore(), svc: &stubAIService{llmStreamFn: func(ctx context.Context, _ string, _ string, _ string, _ []aisvc.ContentItem, _ map[string]interface{}, onDelta func(string)) (string, error) {
				called = true
				actual, ok := ctx.Deadline()
				require.True(t, ok)
				require.Equal(t, expected, actual)
				onDelta("ok")
				return "ok", nil
			}}}
			h.sessions.sessions["fixture"] = &llmSession{TenantUUID: tenant, ModelKey: "ollama/qwen3:8b", Messages: []llmSessionAppendRequest{{Role: "user", Content: []contentItem{{Type: "text", Content: "fixture"}}}}}
			router := gin.New()
			router.GET("/api/v1/ai/llm/sessions/:session_id/stream", h.llmSessionStream)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/ai/llm/sessions/fixture/stream?env=dev", nil).WithContext(parent)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code)
			require.True(t, called)
			require.Contains(t, response.Body.String(), "event: done")
		})
	}
}
