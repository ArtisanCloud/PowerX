package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	agentsvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	aisvc "github.com/ArtisanCloud/PowerX/internal/service/ai"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// testVision 使用当前表单执行管理试跑；复用正式视觉鉴权、固定驱动，不保存配置。
func (h *AgentSettingHandler) testVision(c *gin.Context, tenant string, req testCallReq) {
	if req.Modality != contract.ModVLM || req.VLM == nil {
		respondVisionSettingsError(c, aisvc.MediaError(400, "AI_VISION_TRIAL_REQUEST_INVALID"))
		return
	}
	v := req.VLM
	if h.aiSvc == nil {
		respondVisionSettingsError(c, aisvc.MediaError(503, "AI_DEPENDENCY_UNAVAILABLE"))
		return
	}
	ctx := c.Request.Context()
	if h.visionTrialTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.visionTrialTimeout)
		defer cancel()
	}
	started := time.Now()
	connection := agentsvc.VisionTrialConnection{CurrentForm: req.UseFormConnection || !v.ReuseCredential || v.APIKey != "" || v.BaseURL != "", BaseURL: v.BaseURL, APIKey: v.APIKey, AuthMode: v.AuthMode, Organization: v.Organization, Region: v.Region, SecretID: v.SecretID, SecretKey: v.SecretKey, AzureDeployment: v.AzureDeployment}
	result, err := h.aiSvc.TrialVision(ctx, req.Env, tenant, v.Provider, v.Model, connection, req.Inputs, v.parameters())
	if err != nil {
		h.emitAuditEvent(c, tenant, req.Env, "agent.settings.test_vision", contract.ModVLM, v.Provider, v.Model, false, dto.CodeOf(err))
		respondVisionSettingsError(c, err)
		return
	}
	trace := reqctx.GetTraceID(ctx)
	if trace == "" {
		trace = strings.TrimSpace(c.GetHeader("X-Request-ID"))
	}
	if trace == "" {
		trace = uuid.NewString()
	}
	result["trace_id"], result["elapsed_ms"], result["env"] = trace, time.Since(started).Milliseconds(), req.Env
	h.emitAuditEvent(c, tenant, req.Env, "agent.settings.test_vision", contract.ModVLM, v.Provider, v.Model, true, fmt.Sprintf("analysis completed elapsed_ms=%v trace_id=%s", result["elapsed_ms"], trace))
	dto.ResponseSuccess(c, gin.H{"ok": true, "generation_submitted": true, "result": result})
}
