package ai

import (
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
	svc      *aisvc.Service
	sessions *llmSessionStore
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
	out, err := h.svc.LLMInvoke(
		c.Request.Context(),
		env,
		tenantUUID,
		sess.ModelKey,
		[]aisvc.ContentItem{{Type: "text", Text: prompt}},
		nil,
	)
	if err != nil {
		respondAIError(c, err)
		return
	}

	streamSSE(c, sessionID, out)
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

func streamSSE(c *gin.Context, sessionID, text string) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	writeSSE(c, "start", `{"session_id":"`+sessionID+`"}`)
	writeSSE(c, "token", `{"text":`+jsonString(text)+`}`)
	writeSSE(c, "final", `{"message":`+jsonString(text)+`}`)
	writeSSE(c, "end", `{"ok":true}`)
}

func writeSSE(c *gin.Context, event, data string) {
	if _, err := c.Writer.WriteString("event: " + event + "\n"); err != nil {
		return
	}
	if _, err := c.Writer.WriteString("data: " + data + "\n\n"); err != nil {
		return
	}
	if f, ok := c.Writer.(http.Flusher); ok {
		f.Flush()
	}
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

func pickEnv(c *gin.Context, tenantUUID string, svc *aisvc.Service) (string, error) {
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
	if tenant := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context())); tenant != "" {
		return reqctx.CanonicalTenantUUID(tenant)
	}
	for _, candidate := range []string{
		c.GetHeader("X-PowerX-Tenant"),
		c.Query("tenant_uuid"),
	} {
		value := strings.TrimSpace(candidate)
		if value == "" {
			continue
		}
		return reqctx.CanonicalTenantUUID(value)
	}
	return "", reqctx.ErrTenantUUIDMissing
}
