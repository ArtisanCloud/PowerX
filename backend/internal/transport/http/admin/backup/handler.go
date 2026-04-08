package backup

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	dtoops "github.com/ArtisanCloud/PowerX/internal/dto/ops"
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type handler struct {
	policySvc  *backupops.PolicyService
	jobSvc     *backupops.JobService
	restoreSvc *backupops.RestoreDrillService
}

func NewHandler(deps *shared.Deps) *handler {
	if deps == nil || deps.DB == nil {
		return nil
	}
	return &handler{
		policySvc:  backupops.NewPolicyService(deps.DB),
		jobSvc:     backupops.NewJobService(deps.DB),
		restoreSvc: backupops.NewRestoreDrillService(deps.DB),
	}
}

func (h *handler) ListPolicies(c *gin.Context) {
	enabledOnly := strings.EqualFold(strings.TrimSpace(c.Query("enabled_only")), "true")
	items, total, err := h.policySvc.ListPolicies(c.Request.Context(), backupops.ListPolicyOptions{EnabledOnly: enabledOnly, Page: 1, PageSize: 200})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "list backup policies failed", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items, "pagination": gin.H{"total": total, "page": 1, "page_size": 200}})
}

func (h *handler) UpsertPolicy(c *gin.Context) {
	var req dtoops.BackupPolicyUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	row, err := h.policySvc.UpsertPolicy(c.Request.Context(), backupops.UpsertPolicyRequest{
		Name:          req.Name,
		BackupType:    req.BackupType,
		Schedule:      req.Schedule,
		RetentionDays: req.RetentionDays,
		Enabled:       req.Enabled,
		StorageTarget: req.StorageTarget,
		Operator:      resolveOperator(c),
		TraceID:       strings.TrimSpace(reqctx.GetTraceID(c.Request.Context())),
	})
	if err != nil {
		h.respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"policy": row})
}

func (h *handler) TriggerBackupJob(c *gin.Context) {
	var req dtoops.BackupJobRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	policyID := parseUint(req.PolicyID)
	row, err := h.jobSvc.TriggerJob(c.Request.Context(), backupops.TriggerJobRequest{PolicyID: policyID, Operator: resolveOperator(c), TraceID: strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))})
	if err != nil {
		h.respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"job": row})
}

func (h *handler) ListBackupJobs(c *gin.Context) {
	policyID := parseUint(c.Query("policy_id"))
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parseInt(c.DefaultQuery("page_size", "20"), 20)
	items, total, err := h.jobSvc.ListJobs(c.Request.Context(), backupops.ListJobOptions{PolicyID: policyID, Page: page, PageSize: pageSize})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "list backup jobs failed", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items, "pagination": gin.H{"total": total, "page": page, "page_size": pageSize}})
}

func (h *handler) TriggerCleanup(c *gin.Context) {
	err := h.jobSvc.TriggerCleanup(c.Request.Context(), resolveOperator(c), strings.TrimSpace(reqctx.GetTraceID(c.Request.Context())))
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "trigger cleanup failed", err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"status": "success"})
}

func (h *handler) TriggerRestoreDrill(c *gin.Context) {
	var req dtoops.RestoreDrillRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	sourceJobID := parseUint(req.SourceJobID)
	row, err := h.restoreSvc.Trigger(c.Request.Context(), backupops.TriggerRestoreDrillRequest{SourceJobID: sourceJobID, Operator: resolveOperator(c), TraceID: strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))})
	if err != nil {
		h.respondError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"drill": row})
}

func (h *handler) respondError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, backupops.ErrInvalidBackupPolicy), errors.Is(err, backupops.ErrInvalidBackupRequest), errors.Is(err, backupops.ErrInvalidRestoreDrillRequest):
		dto.ResponseError(c, http.StatusBadRequest, "invalid backup request", err)
	case errors.Is(err, backupops.ErrBackupPolicyNotFound):
		dto.ResponseError(c, http.StatusNotFound, "backup policy not found", err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "backup operation failed", err)
	}
}

func resolveOperator(c *gin.Context) string {
	ctx := c.Request.Context()
	if reqctx.IsRoot(ctx) {
		return "root"
	}
	if reqctx.GetMemberID(ctx) > 0 {
		return "member"
	}
	return "system"
}

func parseInt(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func parseUint(raw string) uint64 {
	v, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
