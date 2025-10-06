package agent

import (
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dto "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
)

// ===== Service holder =====
type AgentSessionHandler struct {
	his *agentSvc.ChatHistoryService
}

func NewAgentSessionHandler(dep *shared.Deps) *AgentSessionHandler {
	return &AgentSessionHandler{
		his: agentSvc.NewChatHistoryService(dep.DB),
	}
}

// ====== Requests/Responses ======

type createSessionReq struct {
	Env       string            `json:"env" validate:"required"`
	AgentID   uint64            `json:"agentId" validate:"required"`
	Title     string            `json:"title"`
	UserID    uint64            `json:"userId"`              // 可选；没有就由后端取鉴权上下文（此处留空也行）
	Singleton *bool             `json:"singleton,omitempty"` // 不传就按 Agent 策略；这里只作直传
	TTLDays   int               `json:"ttlDays,omitempty"`
	MaxKB     int               `json:"maxKB,omitempty"`
	MaxTokens int               `json:"maxTokens,omitempty"`
	Meta      datatypes.JSONMap `json:"meta,omitempty"`
}

type updateSessionReq struct {
	Title     *string `json:"title,omitempty"`
	TTLDays   *int    `json:"ttlDays,omitempty"`
	MaxKB     *int    `json:"maxKB,omitempty"`
	MaxTokens *int    `json:"maxTokens,omitempty"`
}

type appendMsgReq struct {
	Env       string            `json:"env" validate:"required"`
	SessionID uint64            `json:"sessionId" validate:"required"`
	AgentID   uint64            `json:"agentId" validate:"required"`
	Role      string            `json:"role" validate:"required"` // user|assistant|system|tool|summary
	Content   string            `json:"content" validate:"required"`
	Format    string            `json:"format"` // 默认 text
	Tokens    int               `json:"tokens,omitempty"`
	SizeBytes int               `json:"sizeBytes,omitempty"`
	Pinned    bool              `json:"pinned,omitempty"`
	Meta      datatypes.JSONMap `json:"meta,omitempty"`
}

// ====== Handlers ======

// POST /agents/sessions
func (h *AgentSessionHandler) CreateSession(c *gin.Context) {
	var req createSessionReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tid, err := reqctx.RequireTenantIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}

	// 单例标志：如果没传，默认 false（可在上层读取 Agent 配置再传入）
	singleton := false
	if req.Singleton != nil {
		singleton = *req.Singleton
	}

	def := dbmodel.AgentChatSession{
		Title:     strings.TrimSpace(req.Title),
		TTLDays:   req.TTLDays,
		MaxKB:     req.MaxKB,
		MaxTokens: req.MaxTokens,
		Meta:      req.Meta,
	}

	out, err := h.his.GetOrCreateSession(c.Request.Context(), req.Env, &tid, req.AgentID, req.UserID, singleton, &def)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dto.ResponseSuccess(c, out)
}

// GET /agents/sessions?env=...&agent_id=1&status=active,archived&limit=50&offset=0
func (h *AgentSessionHandler) ListSessions(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tid, err := reqctx.RequireTenantIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	agentID, err := utils.ParseUintID(c.DefaultQuery("agent_id", "0"))
	if err != nil || agentID == 0 {
		dto.ResponseError(c, 400, "agent_id 必填", nil)
		return
	}

	var statuses []string
	if s := strings.TrimSpace(c.Query("status")); s != "" {
		for _, it := range strings.Split(s, ",") {
			if v := strings.TrimSpace(it); v != "" {
				statuses = append(statuses, v)
			}
		}
	}
	limit := utils.ParseIntDefault(c.DefaultQuery("limit", "50"), 50)
	offset := utils.ParseIntDefault(c.DefaultQuery("offset", "0"), 0)

	list, err := h.his.ListSessions(c.Request.Context(), env, &tid, agentID, statuses, limit, offset)
	if err != nil {
		dto.ResponseError(c, 500, "查询失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": list})
}

// GET /agents/sessions/:id
func (h *AgentSessionHandler) GetSession(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tid, err := reqctx.RequireTenantIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	sid, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dto.ResponseError(c, 400, "id 非法", nil)
		return
	}

	out, err := h.his.FindSessionByID(c.Request.Context(), env, &tid, sid)
	if err != nil {
		dto.ResponseError(c, 404, "未找到", err)
		return
	}
	dto.ResponseSuccess(c, out)
}

// PATCH /agents/sessions/:id
func (h *AgentSessionHandler) UpdateSession(c *gin.Context) {
	var req updateSessionReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	env := c.DefaultQuery("env", "default")
	tid, err := reqctx.RequireTenantIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	sid, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dto.ResponseError(c, 400, "id 非法", nil)
		return
	}

	if err := h.his.UpdateSessionPolicy(c.Request.Context(), env, &tid, sid, req.Title, req.TTLDays, req.MaxKB, req.MaxTokens); err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// POST /agents/sessions/:id/archive
func (h *AgentSessionHandler) ArchiveSession(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tid, err := reqctx.RequireTenantIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	sid, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dto.ResponseError(c, 400, "id 非法", nil)
		return
	}

	if err := h.his.ArchiveSession(c.Request.Context(), env, &tid, sid); err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// DELETE /agents/sessions/:id
func (h *AgentSessionHandler) DeleteSession(c *gin.Context) {
	// 软删
	sid, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dto.ResponseError(c, 400, "id 非法", nil)
		return
	}
	env := c.DefaultQuery("env", "default")
	tid, err := reqctx.RequireTenantIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	if err := h.his.DeleteSession(c.Request.Context(), env, &tid, sid); err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// GET /agents/sessions/:id/messages?env=...&after_id=0&limit=200
func (h *AgentSessionHandler) ListMessages(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tid, err := reqctx.RequireTenantIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	sid, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dto.ResponseError(c, 400, "id 非法", nil)
		return
	}

	afterID, _ := utils.ParseUint64(c.DefaultQuery("after_id", "0"))
	limit := utils.ParseIntDefault(c.DefaultQuery("limit", "200"), 200)

	list, err := h.his.ListMessages(c.Request.Context(), env, &tid, sid, uint64(afterID), limit)
	if err != nil {
		dto.ResponseError(c, 500, "查询失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": list})
}

// POST /agents/sessions/:id/messages  （便于调试/非流式写入）
func (h *AgentSessionHandler) AppendMessage(c *gin.Context) {
	var req appendMsgReq
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tid, err := reqctx.RequireTenantIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}

	msg, err := h.his.AppendMessage(c.Request.Context(), req.Env, &tid,
		req.SessionID, req.AgentID,
		strings.TrimSpace(req.Role),
		req.Content,
		strings.TrimSpace(req.Format),
		req.Tokens, req.SizeBytes,
		req.Pinned,
		req.Meta,
	)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dto.ResponseSuccess(c, msg)
}

// （可选）触发一次“超限检查+摘要”
func (h *AgentSessionHandler) SummarizeIfNeeded(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tid, err := reqctx.RequireTenantIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	sid, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dto.ResponseError(c, 400, "id 非法", nil)
		return
	}
	sess, err := h.his.FindSessionByID(c.Request.Context(), env, &tid, sid)
	if err != nil {
		dto.ResponseError(c, 404, "未找到", err)
		return
	}

	ok, err := h.his.SummarizeIfNeeded(c.Request.Context(), env, &tid, sess)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dto.ResponseSuccess(c, gin.H{"summarized": ok, "ts": time.Now().Unix()})
}
