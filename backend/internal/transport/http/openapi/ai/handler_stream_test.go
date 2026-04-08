package ai

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	aisvc "github.com/ArtisanCloud/PowerX/internal/service/ai"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

type stubAIService struct {
	llmStreamFn func(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}, onDelta func(string)) (string, error)
}

func (s *stubAIService) LLMInvoke(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}) (*aisvc.LLMInvokeResult, error) {
	return &aisvc.LLMInvokeResult{}, nil
}
func (s *stubAIService) LLMStream(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}, onDelta func(string)) (string, error) {
	if s.llmStreamFn == nil {
		return "", nil
	}
	return s.llmStreamFn(ctx, env, tenantUUID, modelKey, inputs, params, onDelta)
}
func (s *stubAIService) ListLLMModels(ctx context.Context, env string, tenantUUID string, provider string) ([]aisvc.LLMModelItem, error) {
	return nil, nil
}
func (s *stubAIService) ResolveTenantEnv(ctx context.Context, tenantUUID string) (string, bool, error) {
	return "", false, nil
}
func (s *stubAIService) EmbeddingInvoke(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []string, params map[string]interface{}) ([][]float32, error) {
	return nil, nil
}
func (s *stubAIService) ImageInvoke(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
}
func (s *stubAIService) VLMInvoke(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
}
func (s *stubAIService) VideoInvoke(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
}
func (s *stubAIService) TTSInvoke(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}) (map[string]interface{}, error) {
	return nil, nil
}

func TestLLMStreamEndpointSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantUUID := "11111111-1111-1111-1111-111111111111"
	stub := &stubAIService{
		llmStreamFn: func(ctx context.Context, env string, tenant string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}, onDelta func(string)) (string, error) {
			if env != "dev" {
				t.Fatalf("unexpected env: %s", env)
			}
			if tenant != tenantUUID {
				t.Fatalf("unexpected tenant: %s", tenant)
			}
			onDelta("你")
			onDelta("好")
			return "你好", nil
		},
	}
	h := &aiHandler{svc: stub, sessions: newSessionStore()}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(c.Request.Context(), tenantUUID)
		ctx = reqctx.WithEnv(ctx, "dev")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.POST("/api/v1/ai/llm/stream", h.llmStream)

	body := `{"model_key":"openai/gpt-4o-mini","inputs":[{"type":"text","content":"你好"}],"stream_options":{"include_usage":true}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/llm/stream?env=dev", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("unexpected content-type: %s", ct)
	}
	respBody := w.Body.String()
	for _, token := range []string{"event: start", "event: delta", "event: done", `"delta":"你"`, `"delta":"好"`} {
		if !strings.Contains(respBody, token) {
			t.Fatalf("missing %q in body: %s", token, respBody)
		}
	}
}

func TestLLMStreamEndpointSupportsRoleAndContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantUUID := "33333333-3333-3333-3333-333333333333"

	t.Run("role_text_messages", func(t *testing.T) {
		stub := &stubAIService{
			llmStreamFn: func(ctx context.Context, env string, tenant string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}, onDelta func(string)) (string, error) {
				if len(inputs) != 2 {
					t.Fatalf("unexpected input length: %d", len(inputs))
				}
				if inputs[0].Role != "system" || inputs[0].Content != "你是编辑助手" {
					t.Fatalf("unexpected first input: %+v", inputs[0])
				}
				if inputs[1].Role != "user" || inputs[1].Content != "请改写这段文案" {
					t.Fatalf("unexpected second input: %+v", inputs[1])
				}
				onDelta("ok")
				return "ok", nil
			},
		}
		h := &aiHandler{svc: stub, sessions: newSessionStore()}

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := reqctx.WithTenantUUID(c.Request.Context(), tenantUUID)
			ctx = reqctx.WithEnv(ctx, "dev")
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/api/v1/ai/llm/stream", h.llmStream)

		body := `{"model_key":"openai/gpt-4o-mini","inputs":[{"role":"system","type":"text","content":"你是编辑助手"},{"role":"user","type":"text","content":"请改写这段文案"}]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/llm/stream?env=dev", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})

	t.Run("content_only", func(t *testing.T) {
		stub := &stubAIService{
			llmStreamFn: func(ctx context.Context, env string, tenant string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}, onDelta func(string)) (string, error) {
				if len(inputs) != 1 {
					t.Fatalf("unexpected input length: %d", len(inputs))
				}
				if inputs[0].Role != "user" {
					t.Fatalf("unexpected role: %+v", inputs[0])
				}
				if inputs[0].Content != "仅传content字段也应生效" {
					t.Fatalf("content decode failed: %+v", inputs[0])
				}
				onDelta("ok")
				return "ok", nil
			},
		}
		h := &aiHandler{svc: stub, sessions: newSessionStore()}

		r := gin.New()
		r.Use(func(c *gin.Context) {
			ctx := reqctx.WithTenantUUID(c.Request.Context(), tenantUUID)
			ctx = reqctx.WithEnv(ctx, "dev")
			c.Request = c.Request.WithContext(ctx)
			c.Next()
		})
		r.POST("/api/v1/ai/llm/stream", h.llmStream)

		body := `{"model_key":"openai/gpt-4o-mini","inputs":[{"role":"user","content":"仅传content字段也应生效"}]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/llm/stream?env=dev", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
	})
}

func TestLLMSessionStreamEndpointSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tenantUUID := "22222222-2222-2222-2222-222222222222"
	stub := &stubAIService{
		llmStreamFn: func(ctx context.Context, env string, tenant string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}, onDelta func(string)) (string, error) {
			if modelKey != "openai/gpt-4o-mini" {
				t.Fatalf("unexpected model_key: %s", modelKey)
			}
			if len(inputs) == 0 || !strings.Contains(inputs[0].Content, "user: hello") {
				t.Fatalf("unexpected prompt: %+v", inputs)
			}
			onDelta("hel")
			onDelta("lo")
			return "hello", nil
		},
	}
	h := &aiHandler{svc: stub, sessions: newSessionStore()}
	h.sessions.sessions["session-1"] = &llmSession{
		TenantUUID: tenantUUID,
		ModelKey:   "openai/gpt-4o-mini",
		Messages: []llmSessionAppendRequest{
			{
				Role: "user",
				Content: []contentItem{
					{Type: "text", Content: "hello"},
				},
			},
		},
		UpdatedAt: time.Now().UTC(),
	}

	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := reqctx.WithTenantUUID(c.Request.Context(), tenantUUID)
		ctx = reqctx.WithEnv(ctx, "dev")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.GET("/api/v1/ai/llm/sessions/:session_id/stream", h.llmSessionStream)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/llm/sessions/session-1/stream?env=dev", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	respBody := w.Body.String()
	for _, token := range []string{"event: start", `"session_id":"session-1"`, "event: delta", "event: done"} {
		if !strings.Contains(respBody, token) {
			t.Fatalf("missing %q in body: %s", token, respBody)
		}
	}
}
