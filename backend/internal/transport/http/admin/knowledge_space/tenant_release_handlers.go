package knowledge_space

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	tenant_release "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/tenant_release"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
)

// ReleaseHandler exposes tenant release endpoints.
type ReleaseHandler struct {
	svc *tenant_release.Service
}

func NewReleaseHandler(deps *shared.Deps) *ReleaseHandler {
	if deps == nil || deps.KnowledgeSpace == nil || deps.KnowledgeSpace.Release == nil {
		return nil
	}
	return &ReleaseHandler{svc: deps.KnowledgeSpace.Release}
}

type releasePolicyRequest struct {
	MatrixVersion string                     `json:"matrixVersion" binding:"required"`
	PilotTenants  []string                   `json:"pilotTenants"`
	Batches       []tenant_release.BatchSpec `json:"batches" binding:"required,dive"`
	Guardrails    map[string]string          `json:"guardrails"`
	ApprovedBy    string                     `json:"approvedBy"`
	CreatedBy     string                     `json:"createdBy"`
}

type publishRequest struct {
	PolicyID    string `json:"policyId" binding:"required"`
	VersionID   string `json:"versionId" binding:"required"`
	RequestedBy string `json:"requestedBy"`
}

type promoteRequest struct {
	PolicyID    string   `json:"policyId" binding:"required"`
	VersionID   string   `json:"versionId" binding:"required"`
	BatchToken  string   `json:"batchToken" binding:"required"`
	Alerts      []string `json:"alerts"`
	RequestedBy string   `json:"requestedBy"`
}

type rollbackRequest struct {
	PolicyID    string `json:"policyId" binding:"required"`
	VersionID   string `json:"versionId" binding:"required"`
	Reason      string `json:"reason"`
	RequestedBy string `json:"requestedBy"`
}

func (h *ReleaseHandler) UpsertPolicy(c *gin.Context) {
	if !flagEnabled("PX_TENANT_RELEASE_MATRIX") {
		dto.ResponseError(c, http.StatusForbidden, "tenant release matrix disabled", nil)
		return
	}
	var req releasePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	policy, err := h.svc.UpsertPolicy(c.Request.Context(), tenant_release.UpsertPolicyInput{
		MatrixVersion: req.MatrixVersion,
		PilotTenants:  req.PilotTenants,
		Batches:       req.Batches,
		Guardrails:    req.Guardrails,
		ApprovedBy:    req.ApprovedBy,
		CreatedBy:     req.CreatedBy,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{
		"policyId": policy.ID,
		"status":   policy.Status,
	})
}

func (h *ReleaseHandler) Publish(c *gin.Context) {
	if !flagEnabled("PX_KNOWLEDGE_GRAY_RELEASE") {
		dto.ResponseError(c, http.StatusForbidden, "gray release disabled", nil)
		return
	}
	var req publishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	policyID, err := strconv.ParseUint(strings.TrimSpace(req.PolicyID), 10, 64)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid policyId", err)
		return
	}
	res, err := h.svc.Publish(c.Request.Context(), tenant_release.PublishInput{
		PolicyID:    policyID,
		VersionID:   req.VersionID,
		RequestedBy: req.RequestedBy,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"releaseId":  res.ReleaseID,
		"versionId":  res.VersionID,
		"batchToken": res.BatchToken,
		"batchIndex": res.BatchIndex,
		"tenants":    res.Tenants,
	})
}

func (h *ReleaseHandler) Promote(c *gin.Context) {
	if !flagEnabled("PX_KNOWLEDGE_GRAY_RELEASE") {
		dto.ResponseError(c, http.StatusForbidden, "gray release disabled", nil)
		return
	}
	var req promoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	policyID, err := strconv.ParseUint(strings.TrimSpace(req.PolicyID), 10, 64)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid policyId", err)
		return
	}
	result, serr := h.svc.Promote(c.Request.Context(), tenant_release.PromoteInput{
		PolicyID:    policyID,
		VersionID:   req.VersionID,
		BatchToken:  req.BatchToken,
		Alerts:      req.Alerts,
		RequestedBy: req.RequestedBy,
	})
	if serr != nil && !errors.Is(serr, tenant_release.ErrBatchPaused) {
		dto.ResponseError(c, http.StatusInternalServerError, serr.Error(), serr)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"batchToken":     result.BatchToken,
		"batchIndex":     result.BatchIndex,
		"tenants":        result.Tenants,
		"state":          result.State,
		"tenantCoverage": result.TenantCoverage,
	})
}

func (h *ReleaseHandler) Rollback(c *gin.Context) {
	if !flagEnabled("PX_KNOWLEDGE_GRAY_RELEASE") {
		dto.ResponseError(c, http.StatusForbidden, "gray release disabled", nil)
		return
	}
	if !flagEnabled("PX_KNOWLEDGE_RELEASE_GUARD") && strings.TrimSpace(c.GetHeader("X-Bypass-Guard")) == "" {
		// 允许在 guard 关闭时显式绕过（便于 CI/ops 场景控制），否则要求 guard 打开。
		dto.ResponseError(c, http.StatusForbidden, "release guard disabled", nil)
		return
	}
	var req rollbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	policyID, err := strconv.ParseUint(strings.TrimSpace(req.PolicyID), 10, 64)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid policyId", err)
		return
	}
	res, err := h.svc.Rollback(c.Request.Context(), tenant_release.RollbackInput{
		PolicyID:    policyID,
		VersionID:   req.VersionID,
		Reason:      req.Reason,
		RequestedBy: req.RequestedBy,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"status": res.Status,
	})
}

func (h *ReleaseHandler) ListPolicies(c *gin.Context) {
	if !flagEnabled("PX_TENANT_RELEASE_MATRIX") {
		dto.ResponseError(c, http.StatusForbidden, "tenant release matrix disabled", nil)
		return
	}
	limit := 20
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	items, err := h.svc.ListPolicies(c.Request.Context(), limit)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"policies": items})
}

func (h *ReleaseHandler) Status(c *gin.Context) {
	if !flagEnabled("PX_KNOWLEDGE_GRAY_RELEASE") {
		dto.ResponseError(c, http.StatusForbidden, "gray release disabled", nil)
		return
	}
	policyRaw := strings.TrimSpace(c.Query("policyId"))
	versionID := strings.TrimSpace(c.Query("versionId"))
	if policyRaw == "" {
		dto.ResponseError(c, http.StatusBadRequest, "missing policyId", tenant_release.ErrInvalidInput)
		return
	}
	policyID, err := strconv.ParseUint(policyRaw, 10, 64)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid policyId", err)
		return
	}
	if versionID == "" {
		versionID, err = h.svc.LatestVersion(c.Request.Context(), policyID)
		if err != nil {
			dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
			return
		}
		if versionID == "" {
			dto.ResponseSuccess(c, gin.H{"status": nil})
			return
		}
	}
	statusView, err := h.svc.Status(c.Request.Context(), policyID, versionID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"status": statusView})
}
