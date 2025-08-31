package agent

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
	"gorm.io/datatypes"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ====== DTO ======

type AgentHandler struct {
	srv *agentSvc.AgentService
}

func NewAgentHandler(dep *shared.Deps) *AgentHandler {
	return &AgentHandler{
		srv: agentSvc.NewAgentService(dep.DB),
	}
}

type AgentStatusRequest struct {
	AgentID string `form:"agent_id" json:"agent_id,omitempty"` // GET 用 form/query
}

type AgentStatusResponse struct {
	*agentschema.AgentInfo
}

// 仅意图识别的响应体（不要和 dto.ChatData 混用）
type AgentIntentResponse struct {
	Input     string                `json:"input"`
	Timestamp int64                 `json:"timestamp"`
	Intent    *schemas.IntentResult `json:"intent"`
	// 可选：给前端调试/展示
	Debug map[string]any `json:"debug,omitempty"`
}

// ====== Handlers ======

// AgentStatusHandler: 查询 Agent 状态
func (h *AgentHandler) Status(c *gin.Context) {
	var req AgentStatusRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	if strings.TrimSpace(req.AgentID) == "" {
		dtoRequest.ResponseError(c, 400, "agent_id 不能为空", nil)
		return
	}

	mgr := agent.GetAgentManager()
	sysAg, _, rt, err := mgr.Get(req.AgentID)
	if err != nil {
		// Not found 更合适
		dtoRequest.ResponseError(c, 404, "未找到指定的 Agent", err)
		return
	}

	resp := &AgentStatusResponse{
		AgentInfo: &agentschema.AgentInfo{
			AgentID:     sysAg.AgentID,
			Name:        sysAg.Name,
			Description: sysAg.Description,
			Status:      string(sysAg.Status),
			Config:      sysAg.Config,
			CreatedAt:   sysAg.CreatedAt,
			UpdatedAt:   sysAg.UpdatedAt,
			LastBeatAt:  sysAg.LastBeatAt,
			Runtime:     rt,
			Extras:      sysAg.Extras,
		},
	}
	dtoRequest.ResponseSuccess(c, resp)
	return
}

// /api/agents/intent  支持单意图(默认) 或 多任务(?multi=1)
func (h *AgentHandler) Intent(c *gin.Context) {
	var req dtoRequest.ChatRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		dtoRequest.ResponseError(c, 400, "message 不能为空", nil)
		return
	}

	//multi := c.Query("multi") == "1" // ✨ 新增：多任务开关
	multi := true // ✨ 新增：多任务开关
	mgr := agent.GetAgentManager()

	if multi {
		tasks, err := mgr.DetectTasks(c, msg)
		if err != nil {
			dtoRequest.ResponseError(c, 500, "意图识别失败", err)
			return
		}
		dtoRequest.ResponseSuccess(c, gin.H{
			"input": msg,
			"mode":  "intent_multi",
			"tasks": tasks, // 直接返回多任务
			"debug": gin.H{"task_count": len(tasks)},
			"ts":    time.Now().UTC().Unix(),
		})
		return
	}

	// 单意图：原样保留
	intent, err := mgr.DetectIntent(c, msg)
	if err != nil {
		dtoRequest.ResponseError(c, 500, "意图识别失败", err)
		return
	}
	if intent == nil {
		intent = &schemas.IntentResult{Matched: false, Strategy: "none", Reason: "no strategy result"}
	}
	dtoRequest.ResponseSuccess(c, &AgentIntentResponse{
		Input:     msg,
		Timestamp: time.Now().UTC().Unix(),
		Intent:    intent,
		Debug:     map[string]any{"strategy": intent.Strategy, "score": intent.Score, "agent_id": intent.AgentID, "flow_id": intent.FlowID},
	})
	return
}

// /api/agent/intent/plan 仅识别并生成计划（dry-run），不执行
func (h *AgentHandler) PlanPreview(c *gin.Context) {
	// 1) 解析请求
	var req dtoRequest.ChatRequest
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		dtoRequest.ResponseError(c, 400, "message 不能为空", nil)
		return
	}

	// 2) 多意图识别（已内置去重与排序）
	mgr := agent.GetAgentManager()
	tasks, err := mgr.DetectTasks(c, msg)
	if err != nil {
		dtoRequest.ResponseError(c, 500, "意图识别失败", err)
		return
	}
	if len(tasks) == 0 {
		dtoRequest.ResponseSuccess(c, gin.H{
			"mode":   "plan_preview",
			"input":  msg,
			"debug":  gin.H{"task_count": 0},
			"plan":   gin.H{"tasks": []any{}},
			"tasks":  []any{},
			"reason": "no task detected",
		})
		return
	}

	// 3) 基于 YAML 的 requires 自动补全依赖 + 分层 + 参数连线
	plan := mgr.BuildPlan(tasks)

	// 4) 返回
	dtoRequest.ResponseSuccess(c, gin.H{
		"mode":  "plan_preview",
		"input": msg,
		"debug": gin.H{"task_count": len(tasks)},
		"plan":  plan,
		"tasks": tasks,
	})
	return
}

type createAgentReq struct {
	Env              string            `json:"env" validate:"required"`
	Key              string            `json:"key" validate:"required"`
	Name             string            `json:"name" validate:"required"`
	Description      string            `json:"description"`
	Visibility       string            `json:"visibility"` // private|tenant|public
	Status           string            `json:"status"`     // draft|active...
	Scope            string            `json:"scope"`      // system|tenant（语义标签）
	DefaultPersonaID *uint64           `json:"defaultPersonaId"`
	BlueprintRefs    datatypes.JSON    `json:"blueprintRefs"`
	IntentCardsRef   datatypes.JSON    `json:"intentCardsRef"`
	ToolAllowlist    []string          `json:"toolAllowlist"`
	KBStrategy       string            `json:"kbStrategy"`
	Meta             datatypes.JSONMap `json:"meta"`
}

type updateAgentReq struct {
	Name             *string           `json:"name,omitempty"`
	Description      *string           `json:"description,omitempty"`
	Visibility       *string           `json:"visibility,omitempty"`
	Status           *string           `json:"status,omitempty"`
	Scope            *string           `json:"scope,omitempty"`
	DefaultPersonaID *uint64           `json:"defaultPersonaId,omitempty"`
	BlueprintRefs    datatypes.JSON    `json:"blueprintRefs,omitempty"`
	IntentCardsRef   datatypes.JSON    `json:"intentCardsRef,omitempty"`
	ToolAllowlist    []string          `json:"toolAllowlist,omitempty"`
	KBStrategy       *string           `json:"kbStrategy,omitempty"`
	Meta             datatypes.JSONMap `json:"meta,omitempty"`
}

func (h *AgentHandler) CreateAgent(c *gin.Context) {
	var req createAgentReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}

	in := &dbmodel.Agent{
		Env:              req.Env,
		TenantID:         &tid,
		Key:              strings.TrimSpace(req.Key),
		Name:             strings.TrimSpace(req.Name),
		Description:      req.Description,
		Source:           "core",
		Scope:            utils.FirstNonEmpty(req.Scope, "tenant"),
		Visibility:       utils.FirstNonEmpty(req.Visibility, "tenant"),
		Status:           utils.FirstNonEmpty(req.Status, "draft"),
		DefaultPersonaID: req.DefaultPersonaID,
		BlueprintRefs:    req.BlueprintRefs,
		IntentCardsRef:   req.IntentCardsRef,
		ToolAllowlist:    req.ToolAllowlist,
		KBStrategy:       utils.FirstNonEmpty(req.KBStrategy, "union"),
		Meta:             req.Meta,
	}
	out, err := h.srv.Create(c.Request.Context(), req.Env, &tid, in)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dtoRequest.ResponseSuccess(c, out)
}

func (h *AgentHandler) ListAgents(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
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
	list, err := h.srv.List(c.Request.Context(), env, &tid, statuses...)
	if err != nil {
		dtoRequest.ResponseError(c, 500, "查询失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"items": list})
}

func (h *AgentHandler) GetAgent(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	agentID, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dtoRequest.ResponseError(c, 400, "id 非法", nil)
		return
	}
	out, err := h.srv.Get(c.Request.Context(), env, &tid, agentID)
	if err != nil {
		dtoRequest.ResponseError(c, 404, "未找到", err)
		return
	}
	dtoRequest.ResponseSuccess(c, out)
}

func (h *AgentHandler) UpdateAgent(c *gin.Context) {
	var req updateAgentReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	env := c.DefaultQuery("env", "default")
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	agentID, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dtoRequest.ResponseError(c, 400, "id 非法", nil)
		return
	}

	patch := agentSvc.AgentPatch{
		Name:             req.Name,
		Description:      req.Description,
		Visibility:       req.Visibility,
		Status:           req.Status,
		Scope:            req.Scope,
		DefaultPersonaID: req.DefaultPersonaID,
		BlueprintRefs:    req.BlueprintRefs,
		IntentCardsRef:   req.IntentCardsRef,
		ToolAllowlist:    req.ToolAllowlist,
		KBStrategy:       req.KBStrategy,
		Meta:             req.Meta,
	}
	out, err := h.srv.Update(c.Request.Context(), env, &tid, agentID, patch)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dtoRequest.ResponseSuccess(c, out)
}

func (h *AgentHandler) EnableAgent(c *gin.Context)  { h.setAgentStatus(c, dbmodel.AgentStatusActive) }
func (h *AgentHandler) DisableAgent(c *gin.Context) { h.setAgentStatus(c, dbmodel.AgentStatusDisabled) }
func (h *AgentHandler) setAgentStatus(c *gin.Context, status string) {
	env := c.DefaultQuery("env", "default")
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	agentID, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dtoRequest.ResponseError(c, 400, "id 非法", nil)
		return
	}
	if err := h.srv.SetStatus(c.Request.Context(), env, &tid, agentID, status); err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *AgentHandler) DeleteAgent(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	agentID, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dtoRequest.ResponseError(c, 400, "id 非法", nil)
		return
	}
	if err := h.srv.Delete(c.Request.Context(), env, &tid, agentID); err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}

// ====== 管理接口：Agent 级 AI Setting ======

type upsertAgentAISettingReq struct {
	Env           string            `json:"env" validate:"required"`
	Provider      string            `json:"provider"`
	Model         string            `json:"model"`
	Params        datatypes.JSONMap `json:"params"`
	OverrideFlags datatypes.JSONMap `json:"overrideFlags"`
	QuotaPolicy   datatypes.JSONMap `json:"quotaPolicy"`
}

func (h *AgentHandler) GetAgentAISetting(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	agentID, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dtoRequest.ResponseError(c, 400, "id 非法", nil)
		return
	}
	setting, err := h.srv.GetAgentAISetting(c.Request.Context(), env, &tid, agentID)
	if err != nil {
		dtoRequest.ResponseError(c, 404, "未找到", err)
		return
	}
	dtoRequest.ResponseSuccess(c, setting)
}

func (h *AgentHandler) UpsertAgentAISetting(c *gin.Context) {
	var req upsertAgentAISettingReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	agentID, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dtoRequest.ResponseError(c, 400, "id 非法", nil)
		return
	}

	in := &dbmodel.AgentSetting{
		Env:           req.Env,
		AgentID:       agentID,
		Provider:      strings.TrimSpace(req.Provider),
		Model:         strings.TrimSpace(req.Model),
		Params:        req.Params,
		OverrideFlags: req.OverrideFlags,
		QuotaPolicy:   req.QuotaPolicy,
		HealthStatus:  "unknown",
		HealthInfo:    datatypes.JSONMap{},
	}
	out, err := h.srv.UpsertAgentAISetting(c.Request.Context(), req.Env, &tid, in)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dtoRequest.ResponseSuccess(c, out)
}

func (h *AgentHandler) DeleteAgentAISetting(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	agentID, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dtoRequest.ResponseError(c, 400, "id 非法", nil)
		return
	}
	if err := h.srv.DeleteAgentAISetting(c.Request.Context(), env, &tid, agentID); err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true})
}

func (h *AgentHandler) AgentHealthCheck(c *gin.Context) {
	env := c.DefaultQuery("env", "default")
	tid, err := auth.RequireTenantIDFromGin(c)
	if err != nil {
		dtoRequest.ResponseError(c, 400, err.Error(), nil)
		return
	}
	agentID, err := utils.ParseUintID(c.Param("id"))
	if err != nil {
		dtoRequest.ResponseError(c, 400, "id 非法", nil)
		return
	}
	info, err := h.srv.HealthCheck(c.Request.Context(), env, &tid, agentID)
	if err != nil {
		dtoRequest.ResponseError(c, 400, "检查失败", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{"ok": true, "probe": info})
}
