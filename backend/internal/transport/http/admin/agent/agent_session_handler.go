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
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

func isDecimalID(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, ch := range value {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// ===== Service holder =====
type AgentSessionHandler struct {
	his      *agentSvc.ChatHistoryService
	ag       *agentSvc.AgentService
	settings *agentSvc.AgentSettingService
}

func NewAgentSessionHandler(dep *shared.Deps) *AgentSessionHandler {
	return &AgentSessionHandler{
		his:      agentSvc.NewChatHistoryService(dep.DB),
		ag:       agentSvc.NewAgentService(dep.DB),
		settings: agentSvc.NewAgentSettingService(dep.DB),
	}
}

// ====== Requests/Responses ======

type createSessionReq struct {
	Env       string            `json:"env"`
	AgentID   uint64            `json:"agentId"`
	AgentUUID string            `json:"agentUuid"`
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
	Env       string            `json:"env"`
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
	env, err := resolveAgentEnv(c, h.settings)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.Env) == "" || strings.EqualFold(strings.TrimSpace(req.Env), "default") {
		req.Env = env
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	agentID := req.AgentID
	if strings.TrimSpace(req.AgentUUID) != "" {
		agentUUID, err := uuid.Parse(strings.TrimSpace(req.AgentUUID))
		if err != nil {
			dto.ResponseError(c, 400, "agentUuid 非法", err)
			return
		}
		exist, err := h.ag.GetByUUID(c.Request.Context(), req.Env, tenantRef, agentUUID)
		if err != nil {
			dto.ResponseError(c, 404, "未找到指定的 Agent", err)
			return
		}
		agentID = exist.ID
	}
	if agentID == 0 {
		dto.ResponseError(c, 400, "agentId/agentUuid 必填", nil)
		return
	}
	userID := req.UserID
	if userID == 0 {
		userID = reqctx.GetUserID(c.Request.Context())
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

	out, err := h.his.GetOrCreateSession(c.Request.Context(), req.Env, tenantRef, agentID, userID, singleton, &def)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dto.ResponseSuccess(c, out)
}

// GET /agents/sessions?env=...&agent_id=1&status=active,archived&limit=50&offset=0
func (h *AgentSessionHandler) ListSessions(c *gin.Context) {
	env, err := resolveAgentEnv(c, h.settings)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	var agentID uint64
	if agentUUIDStr := strings.TrimSpace(c.Query("agent_uuid")); agentUUIDStr != "" {
		agentUUID, err := uuid.Parse(agentUUIDStr)
		if err != nil {
			dto.ResponseError(c, 400, "agent_uuid 非法", err)
			return
		}
		exist, err := h.ag.GetByUUID(c.Request.Context(), env, tenantRef, agentUUID)
		if err != nil {
			dto.ResponseError(c, 404, "未找到指定的 Agent", err)
			return
		}
		agentID = exist.ID
	} else {
		id, err := utils.ParseUintID(c.DefaultQuery("agent_id", "0"))
		if err != nil || id == 0 {
			dto.ResponseError(c, 400, "agent_uuid/agent_id 必填", nil)
			return
		}
		agentID = id
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

	list, err := h.his.ListSessions(c.Request.Context(), env, tenantRef, agentID, statuses, limit, offset)
	if err != nil {
		dto.ResponseError(c, 500, "查询失败", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": list})
}

// GET /agents/sessions/:id
func (h *AgentSessionHandler) GetSession(c *gin.Context) {
	env, err := resolveAgentEnv(c, h.settings)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	idParam := strings.TrimSpace(c.Param("id"))
	var out *dbmodel.AgentChatSession
	if isDecimalID(idParam) {
		id, parseErr := utils.ParseUintID(idParam)
		if parseErr != nil || id == 0 {
			dto.ResponseError(c, 400, "id 非法", parseErr)
			return
		}
		out, err = h.his.FindSessionByID(c.Request.Context(), env, tenantRef, id)
	} else {
		out, err = h.his.FindSessionByUUID(c.Request.Context(), env, tenantRef, idParam)
	}
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
	env, err := resolveAgentEnv(c, h.settings)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	idParam := strings.TrimSpace(c.Param("id"))
	var sid uint64
	if isDecimalID(idParam) {
		if id, parseErr := utils.ParseUintID(idParam); parseErr == nil && id > 0 {
			sid = id
		}
	} else {
		if sess, findErr := h.his.FindSessionByUUID(c.Request.Context(), env, tenantRef, idParam); findErr == nil && sess != nil {
			sid = sess.ID
		}
	}
	if sid == 0 {
		dto.ResponseError(c, 400, "id 非法", nil)
		return
	}

	if err := h.his.UpdateSessionPolicy(c.Request.Context(), env, tenantRef, sid, req.Title, req.TTLDays, req.MaxKB, req.MaxTokens); err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// POST /agents/sessions/:id/archive
func (h *AgentSessionHandler) ArchiveSession(c *gin.Context) {
	env, err := resolveAgentEnv(c, h.settings)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	idParam := strings.TrimSpace(c.Param("id"))
	var sid uint64
	if isDecimalID(idParam) {
		if id, parseErr := utils.ParseUintID(idParam); parseErr == nil && id > 0 {
			sid = id
		}
	} else {
		if sess, findErr := h.his.FindSessionByUUID(c.Request.Context(), env, tenantRef, idParam); findErr == nil && sess != nil {
			sid = sess.ID
		}
	}
	if sid == 0 {
		dto.ResponseError(c, 400, "id 非法", nil)
		return
	}

	if err := h.his.ArchiveSession(c.Request.Context(), env, tenantRef, sid); err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// DELETE /agents/sessions/:id
func (h *AgentSessionHandler) DeleteSession(c *gin.Context) {
	env, err := resolveAgentEnv(c, h.settings)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	// 软删
	idParam := strings.TrimSpace(c.Param("id"))
	var sid uint64
	if isDecimalID(idParam) {
		if id, parseErr := utils.ParseUintID(idParam); parseErr == nil && id > 0 {
			sid = id
		}
	} else {
		if sess, findErr := h.his.FindSessionByUUID(c.Request.Context(), env, tenantRef, idParam); findErr == nil && sess != nil {
			sid = sess.ID
		}
	}
	if sid == 0 {
		dto.ResponseError(c, 400, "id 非法", nil)
		return
	}
	if err := h.his.DeleteSession(c.Request.Context(), env, tenantRef, sid); err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dto.ResponseSuccess(c, gin.H{"ok": true})
}

// GET /agents/sessions/:id/messages?env=...&after_id=0&limit=200
func (h *AgentSessionHandler) ListMessages(c *gin.Context) {
	env, err := resolveAgentEnv(c, h.settings)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	idParam := strings.TrimSpace(c.Param("id"))
	var sid uint64
	if isDecimalID(idParam) {
		if id, parseErr := utils.ParseUintID(idParam); parseErr == nil && id > 0 {
			sid = id
		}
	} else {
		if sess, findErr := h.his.FindSessionByUUID(c.Request.Context(), env, tenantRef, idParam); findErr == nil && sess != nil {
			sid = sess.ID
		}
	}
	if sid == 0 {
		dto.ResponseError(c, 400, "id 非法", nil)
		return
	}

	afterID, _ := utils.ParseUint64(c.DefaultQuery("after_id", "0"))
	limit := utils.ParseIntDefault(c.DefaultQuery("limit", "200"), 200)

	list, err := h.his.ListMessages(c.Request.Context(), env, tenantRef, sid, uint64(afterID), limit)
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
	env, err := resolveAgentEnv(c, h.settings)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	if strings.TrimSpace(req.Env) == "" || strings.EqualFold(strings.TrimSpace(req.Env), "default") {
		req.Env = env
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()

	msg, err := h.his.AppendMessage(c.Request.Context(), req.Env, tenantRef,
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
	env, err := resolveAgentEnv(c, h.settings)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	tenantRef := tenantCtx.UUIDPtr()
	idParam := strings.TrimSpace(c.Param("id"))
	var sess *dbmodel.AgentChatSession
	if isDecimalID(idParam) {
		id, parseErr := utils.ParseUintID(idParam)
		if parseErr != nil || id == 0 {
			dto.ResponseError(c, 400, "id 非法", parseErr)
			return
		}
		sess, err = h.his.FindSessionByID(c.Request.Context(), env, tenantRef, id)
	} else {
		sess, err = h.his.FindSessionByUUID(c.Request.Context(), env, tenantRef, idParam)
	}
	if err != nil {
		dto.ResponseError(c, 404, "未找到", err)
		return
	}

	ok, err := h.his.SummarizeIfNeeded(c.Request.Context(), env, tenantRef, sess)
	if err != nil {
		dto.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dto.ResponseSuccess(c, gin.H{"summarized": ok, "ts": time.Now().Unix()})
}
