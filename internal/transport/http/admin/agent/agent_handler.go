package agent

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentschema "github.com/ArtisanCloud/PowerX/internal/server/agent/schemas"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ====== DTO ======

type AgentHandler struct {
}

func NewAgentHandler(_ *shared.Deps) *AgentHandler {
	return &AgentHandler{}
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
