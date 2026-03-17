package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	aisvc "github.com/ArtisanCloud/PowerX/internal/service/ai"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type aiHandler struct {
	svc      aiService
	sessions *llmSessionStore
}

type aiService interface {
	LLMInvoke(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}) (string, error)
	LLMStream(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}, onDelta func(string)) (string, error)
	ListLLMModels(ctx context.Context, env string, tenantUUID string, provider string) ([]aisvc.LLMModelItem, error)
	ResolveTenantEnv(ctx context.Context, tenantUUID string) (string, bool, error)
	EmbeddingInvoke(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []string, params map[string]interface{}) ([][]float32, error)
	ImageInvoke(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}) (map[string]interface{}, error)
	VLMInvoke(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}) (map[string]interface{}, error)
	VideoInvoke(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}) (map[string]interface{}, error)
	TTSInvoke(ctx context.Context, env string, tenantUUID string, modelKey string, inputs []aisvc.ContentItem, params map[string]interface{}) (map[string]interface{}, error)
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
	URL  string `json:"url"`
}

type llmInvokeRequest struct {
	ModelKey string                 `json:"model_key"`
	Inputs   []contentItem          `json:"inputs"`
	Params   map[string]interface{} `json:"params"`
}

type llmStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type llmStreamRequest struct {
	ModelKey      string                 `json:"model_key"`
	Inputs        []contentItem          `json:"inputs"`
	Params        map[string]interface{} `json:"params"`
	StreamOptions llmStreamOptions       `json:"stream_options"`
}

type modalInvokeRequest struct {
	ModelKey string                 `json:"model_key"`
	Inputs   []contentItem          `json:"inputs"`
	Params   map[string]interface{} `json:"params"`
}

type llmInvokeResponse struct {
	Output map[string]interface{} `json:"output"`
	Usage  map[string]interface{} `json:"usage,omitempty"`
}

type llmSessionCreateRequest struct {
	ModelKey string `json:"model_key"`
	Title    string `json:"title"`
}

type llmSessionCreateResponse struct {
	SessionID string `json:"session_id"`
}

type llmSessionAppendRequest struct {
	Role    string        `json:"role"`
	Content []contentItem `json:"content"`
}

type llmSession struct {
	TenantUUID string
	ModelKey   string
	Messages   []llmSessionAppendRequest
	UpdatedAt  time.Time
}

type embeddingInvokeRequest struct {
	ModelKey string                 `json:"model_key"`
	Inputs   []string               `json:"inputs"`
	Params   map[string]interface{} `json:"params"`
}

type embeddingInvokeResponse struct {
	Vectors [][]float32         `json:"vectors"`
	Usage   map[string]any      `json:"usage,omitempty"`
	Extra   map[string]string   `json:"extra,omitempty"`
	Meta    map[string]any      `json:"meta,omitempty"`
	Errors  []map[string]string `json:"errors,omitempty"`
}

type llmSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*llmSession
}

func newSessionStore() *llmSessionStore {
	return &llmSessionStore{
		sessions: map[string]*llmSession{},
	}
}

func newAIHandler(deps *shared.Deps) *aiHandler {
	return &aiHandler{
		svc:      aisvc.NewService(deps.DB),
		sessions: newSessionStore(),
	}
}

func (h *aiHandler) notSupported(c *gin.Context) {
	dto.ResponseError(c, http.StatusNotImplemented, "modality not supported yet", nil)
}

func (h *aiHandler) llmInvoke(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "ai service unavailable", nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant uuid missing", err)
		return
	}
	env, err := pickEnv(c, tenantUUID, h.svc)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var req llmInvokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	out, err := h.svc.LLMInvoke(c.Request.Context(), env, tenantUUID, req.ModelKey, toServiceItems(req.Inputs), req.Params)
	if err != nil {
		respondAIError(c, err)
		return
	}

	resp := llmInvokeResponse{
		Output: map[string]interface{}{
			"type": "text",
			"text": out,
		},
	}
	dto.ResponseSuccess(c, resp)
}

func (h *aiHandler) llmStream(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "ai service unavailable", nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant uuid missing", err)
		return
	}
	env, err := pickEnv(c, tenantUUID, h.svc)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var req llmStreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	streamLLM(c, llmStreamMeta{
		TraceID:      uuid.NewString(),
		ModelKey:     strings.TrimSpace(req.ModelKey),
		IncludeUsage: req.StreamOptions.IncludeUsage,
	}, func(onDelta func(string)) (string, error) {
		return h.svc.LLMStream(
			c.Request.Context(),
			env,
			tenantUUID,
			req.ModelKey,
			toServiceItems(req.Inputs),
			req.Params,
			onDelta,
		)
	})
}

func (h *aiHandler) llmModelsList(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "ai service unavailable", nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant uuid missing", err)
		return
	}
	env, err := pickEnv(c, tenantUUID, h.svc)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	provider := strings.TrimSpace(c.Query("provider"))
	items, err := h.svc.ListLLMModels(c.Request.Context(), env, tenantUUID, provider)
	if err != nil {
		dto.ResponseError(c, http.StatusBadGateway, "failed to load llm model list", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"env":   env,
		"items": items,
	})
}

func (h *aiHandler) llmSessionCreate(c *gin.Context) {
	if h == nil || h.sessions == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "ai session unavailable", nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant uuid missing", err)
		return
	}

	var req llmSessionCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}

	modelKey := strings.TrimSpace(req.ModelKey)
	if modelKey == "" {
		dto.ResponseError(c, http.StatusBadRequest, "model_key required", nil)
		return
	}

	sessionID := uuid.NewString()
	h.sessions.mu.Lock()
	h.sessions.sessions[sessionID] = &llmSession{
		TenantUUID: tenantUUID,
		ModelKey:   modelKey,
		UpdatedAt:  time.Now().UTC(),
	}
	h.sessions.mu.Unlock()

	dto.ResponseSuccess(c, llmSessionCreateResponse{SessionID: sessionID})
}

func (h *aiHandler) llmSessionAppend(c *gin.Context) {
	if h == nil || h.sessions == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "ai session unavailable", nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant uuid missing", err)
		return
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "session_id required", nil)
		return
	}

	var req llmSessionAppendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	if strings.TrimSpace(req.Role) == "" {
		dto.ResponseError(c, http.StatusBadRequest, "role required", nil)
		return
	}

	h.sessions.mu.Lock()
	defer h.sessions.mu.Unlock()
	sess, ok := h.sessions.sessions[sessionID]
	if !ok || sess == nil || sess.TenantUUID != tenantUUID {
		dto.ResponseError(c, http.StatusNotFound, "session not found", nil)
		return
	}
	sess.Messages = append(sess.Messages, req)
	sess.UpdatedAt = time.Now().UTC()

	dto.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *aiHandler) llmSessionStream(c *gin.Context) {
	if h == nil || h.svc == nil || h.sessions == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "ai session unavailable", nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant uuid missing", err)
		return
	}
	env, err := pickEnv(c, tenantUUID, h.svc)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "session_id required", nil)
		return
	}

	h.sessions.mu.RLock()
	sess, ok := h.sessions.sessions[sessionID]
	h.sessions.mu.RUnlock()
	if !ok || sess == nil || sess.TenantUUID != tenantUUID {
		dto.ResponseError(c, http.StatusNotFound, "session not found", nil)
		return
	}

	prompt := buildPromptFromSession(sess.Messages)
	if strings.TrimSpace(prompt) == "" {
		dto.ResponseError(c, http.StatusBadRequest, "session empty", nil)
		return
	}
	streamLLM(c, llmStreamMeta{
		TraceID:   uuid.NewString(),
		ModelKey:  sess.ModelKey,
		SessionID: sessionID,
	}, func(onDelta func(string)) (string, error) {
		return h.svc.LLMStream(
			c.Request.Context(),
			env,
			tenantUUID,
			sess.ModelKey,
			[]aisvc.ContentItem{{Type: "text", Text: prompt}},
			nil,
			onDelta,
		)
	})
}

func (h *aiHandler) embeddingInvoke(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "ai service unavailable", nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant uuid missing", err)
		return
	}
	env, err := pickEnv(c, tenantUUID, h.svc)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	var req embeddingInvokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	vecs, err := h.svc.EmbeddingInvoke(c.Request.Context(), env, tenantUUID, req.ModelKey, req.Inputs, req.Params)
	if err != nil {
		respondAIError(c, err)
		return
	}
	dto.ResponseSuccess(c, embeddingInvokeResponse{Vectors: vecs})
}

func (h *aiHandler) imageInvoke(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "ai service unavailable", nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant uuid missing", err)
		return
	}
	env, err := pickEnv(c, tenantUUID, h.svc)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	var req modalInvokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	out, err := h.svc.ImageInvoke(c.Request.Context(), env, tenantUUID, req.ModelKey, toServiceItems(req.Inputs), req.Params)
	if err != nil {
		respondAIError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *aiHandler) vlmInvoke(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "ai service unavailable", nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant uuid missing", err)
		return
	}
	env, err := pickEnv(c, tenantUUID, h.svc)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	var req modalInvokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	out, err := h.svc.VLMInvoke(c.Request.Context(), env, tenantUUID, req.ModelKey, toServiceItems(req.Inputs), req.Params)
	if err != nil {
		respondAIError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *aiHandler) videoInvoke(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "ai service unavailable", nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant uuid missing", err)
		return
	}
	env, err := pickEnv(c, tenantUUID, h.svc)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	var req modalInvokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	out, err := h.svc.VideoInvoke(c.Request.Context(), env, tenantUUID, req.ModelKey, toServiceItems(req.Inputs), req.Params)
	if err != nil {
		respondAIError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *aiHandler) ttsInvoke(c *gin.Context) {
	if h == nil || h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "ai service unavailable", nil)
		return
	}
	tenantUUID, err := tenantUUIDFromRequest(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant uuid missing", err)
		return
	}
	env, err := pickEnv(c, tenantUUID, h.svc)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	var req modalInvokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	out, err := h.svc.TTSInvoke(c.Request.Context(), env, tenantUUID, req.ModelKey, toServiceItems(req.Inputs), req.Params)
	if err != nil {
		respondAIError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

type llmStreamMeta struct {
	TraceID      string
	ModelKey     string
	SessionID    string
	IncludeUsage bool
}

func streamLLM(c *gin.Context, meta llmStreamMeta, invoke func(onDelta func(string)) (string, error)) {
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	start := map[string]interface{}{
		"traceId":   meta.TraceID,
		"model_key": meta.ModelKey,
	}
	if meta.SessionID != "" {
		start["session_id"] = meta.SessionID
	}
	if err := writeSSEJSON(c, "start", start); err != nil {
		return
	}

	index := 0
	final, err := invoke(func(delta string) {
		_ = writeSSEJSON(c, "delta", map[string]interface{}{
			"delta":   delta,
			"index":   index,
			"traceId": meta.TraceID,
		})
		index++
	})
	if err != nil {
		_ = writeSSEJSON(c, "error", map[string]interface{}{
			"code":    "upstream_error",
			"message": err.Error(),
			"traceId": meta.TraceID,
		})
		return
	}

	if meta.IncludeUsage {
		_ = writeSSEJSON(c, "usage", map[string]interface{}{
			"traceId": meta.TraceID,
		})
	}
	_ = writeSSEJSON(c, "done", map[string]interface{}{
		"ok":            true,
		"text":          final,
		"finish_reason": "stop",
		"traceId":       meta.TraceID,
	})
}

func writeSSE(c *gin.Context, event, data string) {
	_ = writeSSEPayload(c, event, data)
}

func writeSSEJSON(c *gin.Context, event string, v interface{}) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return writeSSEPayload(c, event, string(raw))
}

func writeSSEPayload(c *gin.Context, event, data string) error {
	if _, err := c.Writer.WriteString("event: " + event + "\n"); err != nil {
		return err
	}
	if _, err := c.Writer.WriteString("data: " + data + "\n\n"); err != nil {
		return err
	}
	if f, ok := c.Writer.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

func jsonString(v string) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

func buildPrompt(items []contentItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Type), "text") && strings.TrimSpace(item.Text) != "" {
			parts = append(parts, strings.TrimSpace(item.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func buildPromptFromSession(msgs []llmSessionAppendRequest) string {
	if len(msgs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		text := buildPrompt(msg.Content)
		if text == "" {
			continue
		}
		parts = append(parts, msg.Role+": "+text)
	}
	return strings.Join(parts, "\n")
}

func toServiceItems(items []contentItem) []aisvc.ContentItem {
	if len(items) == 0 {
		return nil
	}
	out := make([]aisvc.ContentItem, 0, len(items))
	for _, item := range items {
		out = append(out, aisvc.ContentItem{
			Type: item.Type,
			Text: item.Text,
			URL:  item.URL,
		})
	}
	return out
}

func respondAIError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	switch err {
	case aisvc.ErrInvalidModelKey:
		dto.ResponseError(c, http.StatusBadRequest, "invalid model_key", err)
	case aisvc.ErrModelNotConfigured:
		dto.ResponseError(c, http.StatusBadRequest, "model not configured for tenant", err)
	case aisvc.ErrPromptRequired:
		dto.ResponseError(c, http.StatusBadRequest, "inputs must include text", err)
	case aisvc.ErrProviderUnsupported:
		dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{
			"status": "accepted",
			"reason": "provider not supported yet",
		})
	default:
		dto.ResponseError(c, http.StatusBadGateway, "ai invoke failed", err)
	}
}

type envResolver interface {
	ResolveTenantEnv(ctx context.Context, tenantUUID string) (string, bool, error)
}

func pickEnv(c *gin.Context, tenantUUID string, svc envResolver) (string, error) {
	if c == nil {
		return "", errors.New("env missing")
	}
	env := strings.TrimSpace(reqctx.GetEnv(c.Request.Context()))
	if strings.EqualFold(env, "default") {
		env = ""
	}
	if env == "" {
		if v := strings.TrimSpace(c.Query("env")); v != "" && !strings.EqualFold(v, "default") {
			env = v
		}
	}
	if env == "" {
		if v := strings.TrimSpace(c.GetHeader("X-PowerX-Env")); v != "" && !strings.EqualFold(v, "default") {
			env = v
		}
	}
	if env == "" && svc != nil {
		if v, ok, _ := svc.ResolveTenantEnv(c.Request.Context(), tenantUUID); ok {
			env = v
		}
	}
	if strings.TrimSpace(env) == "" {
		return "", errors.New("env missing")
	}
	return env, nil
}

func tenantUUIDFromRequest(c *gin.Context) (string, error) {
	if c == nil {
		return "", reqctx.ErrTenantUUIDMissing
	}
	tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	if tenant == "" {
		return "", reqctx.ErrTenantUUIDMissing
	}
	return reqctx.CanonicalTenantUUID(tenant)
}
