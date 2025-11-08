package plugin_release

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/pipeline"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/plugin_release"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type releaseGuardrailHandler struct {
	pipeline *pipeline.Service
}

func newReleaseGuardrailHandler(p *pipeline.Service) *releaseGuardrailHandler {
	if p == nil {
		return nil
	}
	return &releaseGuardrailHandler{pipeline: p}
}

type createCandidateRequest struct {
	TenantID     string            `json:"tenantId" binding:"required"`
	PluginID     string            `json:"pluginId" binding:"required"`
	Version      string            `json:"version" binding:"required"`
	ArtifactURI  string            `json:"buildArtifactUri" binding:"required"`
	CommitHash   string            `json:"commitHash" binding:"required"`
	ReleaseNotes string            `json:"releaseNotes" binding:"required"`
	Labels       map[string]string `json:"labels"`
}

type runGateResponse struct {
	CandidateID string                   `json:"candidateId"`
	Status      string                   `json:"status"`
	Violations  []pipeline.GateViolation `json:"violations,omitempty"`
}

type createPlanRequest struct {
	CandidateID         string                      `json:"releaseCandidateId" binding:"required"`
	WindowStart         string                      `json:"windowStart" binding:"required"`
	WindowEnd           string                      `json:"windowEnd" binding:"required"`
	CanaryBatches       []pipeline.CanaryBatchInput `json:"canaryBatches" binding:"required,dive"`
	RollbackScripts     []string                    `json:"rollbackScripts"`
	NotificationTargets []string                    `json:"notificationTargets"`
	DependencyList      []string                    `json:"dependencyList"`
}

func (h *releaseGuardrailHandler) createCandidate(c *gin.Context) {
	if h.pipeline == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "pipeline unavailable", nil)
		return
	}
	var req createCandidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	candidate, err := h.pipeline.SubmitCandidate(c.Request.Context(), pipeline.SubmitCandidateInput{
		TenantID:      strings.TrimSpace(req.TenantID),
		PluginID:      strings.TrimSpace(req.PluginID),
		Version:       strings.TrimSpace(req.Version),
		BuildArtifact: strings.TrimSpace(req.ArtifactURI),
		CommitHash:    strings.TrimSpace(req.CommitHash),
		ReleaseNotes:  strings.TrimSpace(req.ReleaseNotes),
		Labels:        req.Labels,
		Actor:         c.GetHeader("Authorization"),
	})
	if err != nil {
		writePipelineError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toCandidateResponse(candidate))
}

func (h *releaseGuardrailHandler) getCandidate(c *gin.Context) {
	if h.pipeline == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "pipeline unavailable", nil)
		return
	}
	candidateUUID, err := uuid.Parse(strings.TrimSpace(c.Param("candidateId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid candidateId", err)
		return
	}
	candidate, err := h.pipeline.GetCandidate(c.Request.Context(), candidateUUID)
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "get candidate failed", err)
		return
	}
	if candidate == nil {
		dto.ResponseError(c, http.StatusNotFound, "candidate not found", nil)
		return
	}
	dto.ResponseSuccess(c, toCandidateResponse(candidate))
}

func (h *releaseGuardrailHandler) runGates(c *gin.Context) {
	if h.pipeline == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "pipeline unavailable", nil)
		return
	}
	candidateUUID, err := uuid.Parse(strings.TrimSpace(c.Param("candidateId")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid candidateId", err)
		return
	}
	result, err := h.pipeline.RunQualityGates(c.Request.Context(), pipeline.RunQualityGatesInput{
		CandidateID: candidateUUID,
		Actor:       c.GetHeader("Authorization"),
	})
	if err != nil {
		writePipelineError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, runGateResponse{
		CandidateID: result.CandidateID.String(),
		Status:      result.Status,
		Violations:  result.Violations,
	})
}

func (h *releaseGuardrailHandler) createPlan(c *gin.Context) {
	if h.pipeline == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "pipeline unavailable", nil)
		return
	}
	var req createPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	candidateUUID, err := uuid.Parse(strings.TrimSpace(req.CandidateID))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid releaseCandidateId", err)
		return
	}
	windowStart, err := time.Parse(time.RFC3339, req.WindowStart)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid windowStart", err)
		return
	}
	windowEnd, err := time.Parse(time.RFC3339, req.WindowEnd)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid windowEnd", err)
		return
	}
	plan, candidate, err := h.pipeline.GenerateReleasePlan(c.Request.Context(), pipeline.GeneratePlanInput{
		CandidateID:         candidateUUID,
		WindowStart:         windowStart,
		WindowEnd:           windowEnd,
		CanaryBatches:       req.CanaryBatches,
		RollbackScripts:     req.RollbackScripts,
		NotificationTargets: req.NotificationTargets,
		DependencyList:      req.DependencyList,
		Actor:               c.GetHeader("Authorization"),
	})
	if err != nil {
		writePipelineError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toPlanResponse(plan, candidate))
}

type candidateResponse struct {
	ID             string            `json:"candidateId"`
	TenantID       string            `json:"tenantId"`
	PluginID       string            `json:"pluginId"`
	Version        string            `json:"version"`
	BuildArtifact  string            `json:"buildArtifactUri"`
	CommitHash     string            `json:"commitHash"`
	ReleaseNotes   string            `json:"releaseNotes"`
	GateStatus     string            `json:"gateStatus"`
	ApprovalStatus string            `json:"approvalStatus"`
	SubmittedAt    string            `json:"submittedAt,omitempty"`
	GatesCheckedAt string            `json:"gatesCheckedAt,omitempty"`
	ApprovedAt     string            `json:"approvedAt,omitempty"`
	Labels         map[string]string `json:"labels,omitempty"`
}

func toCandidateResponse(candidate *models.PluginReleaseCandidate) candidateResponse {
	resp := candidateResponse{
		ID:             candidate.UUID.String(),
		TenantID:       candidate.TenantID,
		PluginID:       candidate.PluginID,
		Version:        candidate.Version,
		BuildArtifact:  candidate.BuildArtifactURI,
		CommitHash:     candidate.CommitHash,
		ReleaseNotes:   candidate.ReleaseNotes,
		GateStatus:     candidate.GateStatus,
		ApprovalStatus: candidate.ApprovalStatus,
	}
	if candidate.SubmittedAt != nil {
		resp.SubmittedAt = candidate.SubmittedAt.UTC().Format(time.RFC3339)
	}
	if candidate.GatesCheckedAt != nil {
		resp.GatesCheckedAt = candidate.GatesCheckedAt.UTC().Format(time.RFC3339)
	}
	if candidate.ApprovedAt != nil {
		resp.ApprovedAt = candidate.ApprovedAt.UTC().Format(time.RFC3339)
	}
	resp.Labels = decodeStringMap(candidate.Labels)
	return resp
}

type planResponse struct {
	ID                  uint64           `json:"planId"`
	ReleaseCandidateID  string           `json:"releaseCandidateId"`
	Status              string           `json:"status"`
	WindowStart         string           `json:"windowStart"`
	WindowEnd           string           `json:"windowEnd"`
	CanaryBatches       []map[string]any `json:"canaryBatches"`
	RollbackScripts     []string         `json:"rollbackScripts"`
	NotificationTargets []string         `json:"notificationTargets"`
	DependencyList      []string         `json:"dependencyList"`
}

func toPlanResponse(plan *models.ReleasePlan, candidate *models.PluginReleaseCandidate) planResponse {
	candidateID := ""
	if candidate != nil {
		candidateID = candidate.UUID.String()
	}
	return planResponse{
		ID:                  plan.ID,
		ReleaseCandidateID:  candidateID,
		Status:              plan.Status,
		WindowStart:         plan.WindowStart.UTC().Format(time.RFC3339),
		WindowEnd:           plan.WindowEnd.UTC().Format(time.RFC3339),
		CanaryBatches:       decodeMapArray(plan.CanaryBatches),
		RollbackScripts:     decodeStringSlice(plan.RollbackScripts),
		NotificationTargets: decodeStringSlice(plan.NotificationTargets),
		DependencyList:      decodeStringSlice(plan.DependencyList),
	}
}

func decodeStringMap(raw datatypes.JSON) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var payload map[string]string
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload
}

func decodeStringSlice(raw datatypes.JSON) []string {
	if len(raw) == 0 {
		return nil
	}
	var payload []string
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload
}

func decodeMapArray(raw datatypes.JSON) []map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var payload []map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	return payload
}

func writePipelineError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, pipeline.ErrInvalidInput):
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
	case errors.Is(err, pipeline.ErrCandidateNotFound):
		dto.ResponseError(c, http.StatusNotFound, err.Error(), err)
	case errors.Is(err, pipeline.ErrPlanWindowInvalid):
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
	case errors.Is(err, pipeline.ErrGateNotPassed):
		dto.ResponseError(c, http.StatusPreconditionFailed, err.Error(), err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "pipeline operation failed", err)
	}
}
