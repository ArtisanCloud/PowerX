package skills

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type tenantHandler struct {
	invokeSvc  *skillservice.InvokeService
	adapterSvc *skillservice.AdapterService
}

type directInvokeRequest struct {
	SkillID string                 `json:"skill_id"`
	Version string                 `json:"version"`
	Payload map[string]interface{} `json:"payload"`
	Context map[string]interface{} `json:"context"`
}

type unifiedInvokeRequest struct {
	CapabilityID      string                 `json:"capability_id"`
	PreferredProtocol string                 `json:"preferred_protocol"`
	ToolGrantIDs      []string               `json:"tool_grant_ids"`
	Context           map[string]interface{} `json:"context"`
	Payload           map[string]interface{} `json:"payload"`
}

func newTenantHandler(deps *shared.Deps) *tenantHandler {
	if deps == nil || deps.DB == nil {
		return nil
	}
	registryRepo := skillrepo.NewSkillRegistryRepository(deps.DB)
	bindingRepo := skillrepo.NewSkillCapabilityBindingRepository(deps.DB)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(deps.DB)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(deps.DB)
	auditSvc := skillservice.NewAuditTraceService(traceRepo, auditRepo)
	invokeSvc := skillservice.NewInvokeService(registryRepo, auditSvc)
	return &tenantHandler{
		invokeSvc: invokeSvc,
		adapterSvc: skillservice.NewAdapterService(invokeSvc, bindingRepo).
			WithSourcePolicyResolver(skillservice.NewDBSourcePolicyResolver(deps.DB)),
	}
}

func (h *tenantHandler) InvokeDirect(c *gin.Context) {
	var req directInvokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	executed, err := h.invokeSvc.Execute(c.Request.Context(), skillservice.InvokeRequest{
		TenantUUID: tenantUUID,
		SkillID:    req.SkillID,
		Version:    req.Version,
		InvokePath: "tenant.skills.invoke",
	}, req.Payload, req.Context)
	if err != nil {
		code, envelope := skillservice.MapInvokeError(err)
		c.JSON(code, envelope)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"trace_id":      executed.TraceID,
		"status":        executed.Status,
		"protocol_used": executed.ProtocolUsed,
		"fallback_used": executed.FallbackUsed,
		"result":        executed.Result,
	})
}

// InvokeUnified handles preferred_protocol=skill requests.
func (h *tenantHandler) InvokeUnified(c *gin.Context) bool {
	var req unifiedInvokeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return true
	}
	if strings.TrimSpace(strings.ToLower(req.PreferredProtocol)) != "skill" {
		return false
	}
	tenantUUID := strings.TrimSpace(reqctx.GetTenantUUID(c.Request.Context()))
	result, err := h.adapterSvc.InvokeUnified(c.Request.Context(), skillservice.UnifiedInvokeRequest{
		TenantUUID:        tenantUUID,
		Env:               strings.TrimSpace(reqctx.GetEnv(c.Request.Context())),
		CapabilityID:      req.CapabilityID,
		PreferredProtocol: req.PreferredProtocol,
		ToolGrantIDs:      req.ToolGrantIDs,
		Context:           req.Context,
		Payload:           req.Payload,
	})
	if err != nil {
		code, envelope := skillservice.MapInvokeError(err)
		c.JSON(code, envelope)
		return true
	}
	dto.ResponseSuccess(c, gin.H{
		"trace_id":         result.TraceID,
		"status":           result.Status,
		"protocol_used":    result.ProtocolUsed,
		"fallback_used":    result.FallbackUsed,
		"result":           result.Result,
		"skill_id":         result.SkillID,
		"version":          result.Version,
		"skill_candidates": result.SkillCandidates,
	})
	return true
}
