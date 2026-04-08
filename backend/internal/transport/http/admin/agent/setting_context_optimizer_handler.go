package agent

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	appcfg "github.com/ArtisanCloud/PowerX/config"
	agent "github.com/ArtisanCloud/PowerX/internal/server/agent"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	dtoRequest "github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type saveContextOptimizerDraftReq struct {
	Env          string                                 `json:"env" validate:"required"`
	Scope        string                                 `json:"scope"`
	Config       agentSvc.ContextOptimizerConfigPayload `json:"config" validate:"required"`
	ChangeReason string                                 `json:"change_reason"`
}

type publishContextOptimizerReq struct {
	Env          string `json:"env" validate:"required"`
	Scope        string `json:"scope"`
	Version      int    `json:"version" validate:"required"`
	ChangeReason string `json:"change_reason"`
}

type rollbackContextOptimizerReq struct {
	Env           string `json:"env" validate:"required"`
	Scope         string `json:"scope"`
	TargetVersion int    `json:"target_version" validate:"required"`
	ChangeReason  string `json:"change_reason"`
}

func (h *AgentSettingHandler) resolveContextOptimizerScope(c *gin.Context, scope string) (*string, string, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "system" {
		if !reqctx.IsRoot(c.Request.Context()) {
			return nil, "", errors.New("forbidden: system scope requires root")
		}
		return nil, "system", nil
	}
	tenantCtx, err := requireTenantContext(c)
	if err != nil {
		return nil, "", err
	}
	return tenantCtx.UUIDPtr(), "tenant", nil
}

func (h *AgentSettingHandler) getContextOptimizerActive(c *gin.Context) {
	env := strings.TrimSpace(c.DefaultQuery("env", "dev"))
	scope := strings.TrimSpace(c.DefaultQuery("scope", "tenant"))
	tenantRef, finalScope, err := h.resolveContextOptimizerScope(c, scope)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusForbidden, err.Error(), err)
		return
	}
	fallback := agent.GetAgentManager().GetContextOptimizerConfig()
	global := appcfg.GetGlobalConfig()
	debugEnabled := false
	if global != nil {
		debugEnabled = global.LogConfig.AgentDebug.Enable
	}
	view, err := h.ctxOptSvc.GetActive(c.Request.Context(), env, tenantRef, fallback, debugEnabled)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "query context optimizer active failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"env":    env,
		"scope":  finalScope,
		"active": view,
	})
}

func (h *AgentSettingHandler) listContextOptimizerVersions(c *gin.Context) {
	env := strings.TrimSpace(c.DefaultQuery("env", "dev"))
	scope := strings.TrimSpace(c.DefaultQuery("scope", "tenant"))
	limit, _ := strconv.Atoi(strings.TrimSpace(c.DefaultQuery("limit", "50")))
	tenantRef, finalScope, err := h.resolveContextOptimizerScope(c, scope)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusForbidden, err.Error(), err)
		return
	}
	items, err := h.ctxOptSvc.ListVersions(c.Request.Context(), env, finalScope, tenantRef, limit)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusInternalServerError, "query context optimizer versions failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"env":      env,
		"scope":    finalScope,
		"versions": items,
	})
}

func (h *AgentSettingHandler) saveContextOptimizerDraft(c *gin.Context) {
	var req saveContextOptimizerDraftReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tenantRef, finalScope, err := h.resolveContextOptimizerScope(c, req.Scope)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusForbidden, err.Error(), err)
		return
	}
	actor := reqctx.GetUserID(c.Request.Context())
	row, err := h.ctxOptSvc.SaveDraft(c.Request.Context(), strings.TrimSpace(req.Env), finalScope, tenantRef, req.Config, req.ChangeReason, actor)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "save context optimizer draft failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"env":   req.Env,
		"scope": finalScope,
		"draft": row,
	})
}

func (h *AgentSettingHandler) publishContextOptimizer(c *gin.Context) {
	var req publishContextOptimizerReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tenantRef, finalScope, err := h.resolveContextOptimizerScope(c, req.Scope)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusForbidden, err.Error(), err)
		return
	}
	actor := reqctx.GetUserID(c.Request.Context())
	row, err := h.ctxOptSvc.Publish(c.Request.Context(), strings.TrimSpace(req.Env), finalScope, tenantRef, req.Version, req.ChangeReason, actor)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "publish context optimizer config failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"env":       req.Env,
		"scope":     finalScope,
		"published": row,
	})
}

func (h *AgentSettingHandler) rollbackContextOptimizer(c *gin.Context) {
	var req rollbackContextOptimizerReq
	if err := dtoRequest.ValidateRequestWithContext(c, &req); err != nil {
		dtoRequest.ResponseValidationError(c, err)
		return
	}
	tenantRef, finalScope, err := h.resolveContextOptimizerScope(c, req.Scope)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusForbidden, err.Error(), err)
		return
	}
	actor := reqctx.GetUserID(c.Request.Context())
	row, err := h.ctxOptSvc.Rollback(c.Request.Context(), strings.TrimSpace(req.Env), finalScope, tenantRef, req.TargetVersion, req.ChangeReason, actor)
	if err != nil {
		dtoRequest.ResponseError(c, http.StatusBadRequest, "rollback context optimizer config failed", err)
		return
	}
	dtoRequest.ResponseSuccess(c, gin.H{
		"env":         req.Env,
		"scope":       finalScope,
		"rolled_back": row,
	})
}
