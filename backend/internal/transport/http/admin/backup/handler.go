package backup

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	backupdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/backup/dto"
	modelops "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type handler struct {
	policySvc  *backupops.PolicyService
	jobSvc     *backupops.JobService
	restoreSvc *backupops.RestoreDrillService
	alertSvc   *backupops.AlertService
}

func NewHandler(deps *shared.Deps) *handler {
	if deps == nil || deps.DB == nil {
		return nil
	}
	return &handler{
		policySvc:  backupops.NewPolicyService(deps.DB),
		jobSvc:     backupops.NewJobService(deps.DB),
		restoreSvc: backupops.NewRestoreDrillService(deps.DB),
		alertSvc:   backupops.NewAlertService(deps.DB),
	}
}

func (h *handler) ListPolicies(c *gin.Context) {
	enabledOnly := strings.EqualFold(strings.TrimSpace(c.Query("enabled_only")), "true")
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parseInt(c.DefaultQuery("page_size", "20"), 20)
	items, total, err := h.policySvc.ListPolicies(c.Request.Context(), backupops.ListPolicyOptions{
		EnabledOnly: enabledOnly,
		Status:      strings.TrimSpace(c.Query("status")),
		Keyword:     strings.TrimSpace(c.Query("keyword")),
		Timezone:    strings.TrimSpace(c.Query("timezone")),
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items, "pagination": gin.H{"total": total, "page": page, "page_size": pageSize}})
}

func (h *handler) CreatePolicy(c *gin.Context) {
	var req backupdto.BackupPolicyUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	row, err := h.policySvc.CreatePolicy(c.Request.Context(), backupops.CreatePolicyRequest{
		Name:             req.Name,
		IntervalHours:    req.IntervalHours,
		RetentionCount:   req.RetentionCount,
		Timezone:         req.Timezone,
		DrillEnabled:     req.DrillEnabled,
		DrillIntervalDay: req.DrillIntervalDays,
		TargetRef:        req.TargetRef,
		Operator:         resolveOperator(c),
		TraceID:          strings.TrimSpace(reqctx.GetTraceID(c.Request.Context())),
	})
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"policy": row})
}

func (h *handler) UpdatePolicy(c *gin.Context) {
	var req backupdto.BackupPolicyUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	policyID := parseUint(c.Param("policy_id"))
	row, err := h.policySvc.UpdatePolicy(c.Request.Context(), backupops.UpdatePolicyRequest{
		PolicyID:         policyID,
		Name:             req.Name,
		IntervalHours:    req.IntervalHours,
		RetentionCount:   req.RetentionCount,
		Timezone:         req.Timezone,
		DrillEnabled:     req.DrillEnabled,
		DrillIntervalDay: req.DrillIntervalDays,
		TargetRef:        req.TargetRef,
		Operator:         resolveOperator(c),
		TraceID:          strings.TrimSpace(reqctx.GetTraceID(c.Request.Context())),
	})
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"policy": row})
}

func (h *handler) EnablePolicy(c *gin.Context) {
	policyID := parseUint(c.Param("policy_id"))
	err := h.policySvc.SetPolicyEnabled(c.Request.Context(), backupops.SetPolicyEnabledRequest{
		PolicyID: policyID,
		Enabled:  true,
		Operator: resolveOperator(c),
		TraceID:  strings.TrimSpace(reqctx.GetTraceID(c.Request.Context())),
	})
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"policy_id": policyID, "enabled": true})
}

func (h *handler) DisablePolicy(c *gin.Context) {
	policyID := parseUint(c.Param("policy_id"))
	err := h.policySvc.SetPolicyEnabled(c.Request.Context(), backupops.SetPolicyEnabledRequest{
		PolicyID: policyID,
		Enabled:  false,
		Operator: resolveOperator(c),
		TraceID:  strings.TrimSpace(reqctx.GetTraceID(c.Request.Context())),
	})
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"policy_id": policyID, "enabled": false})
}

func (h *handler) TriggerBackupJob(c *gin.Context) {
	var req backupdto.BackupJobRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	policyID := parseUint(req.PolicyID)
	row, err := h.jobSvc.TriggerJob(c.Request.Context(), backupops.TriggerJobRequest{PolicyID: policyID, Operator: resolveOperator(c), TraceID: strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))})
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	logger.Info(c.Request.Context(), "backup.api.trigger_job",
		zap.Uint64("policy_id", policyID),
		zap.Uint64("job_id", row.ID),
		zap.String("status", string(row.Status)),
		zap.String("trace_id", strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))),
	)
	dto.ResponseSuccess(c, gin.H{"job": row})
}

func (h *handler) ListBackupJobs(c *gin.Context) {
	policyID := parseUint(c.Query("policy_id"))
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parseInt(c.DefaultQuery("page_size", "20"), 20)
	status := strings.TrimSpace(c.Query("status"))
	from, fromErr := parseDateTime(c.Query("from"))
	if fromErr != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid from datetime", fromErr)
		return
	}
	to, toErr := parseDateTime(c.Query("to"))
	if toErr != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid to datetime", toErr)
		return
	}
	items, total, err := h.jobSvc.ListJobs(c.Request.Context(), backupops.ListJobOptions{
		PolicyID: policyID,
		Status:   status,
		From:     from,
		To:       to,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items, "pagination": gin.H{"total": total, "page": page, "page_size": pageSize}})
}

func (h *handler) GetBackupJob(c *gin.Context) {
	jobID := parseUint(c.Param("job_id"))
	row, err := h.jobSvc.GetJob(c.Request.Context(), jobID)
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"job": buildJobDetailResponse(row)})
}

func (h *handler) TriggerCleanup(c *gin.Context) {
	err := h.jobSvc.TriggerCleanup(c.Request.Context(), resolveOperator(c), strings.TrimSpace(reqctx.GetTraceID(c.Request.Context())))
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "trigger cleanup failed", err)
		return
	}
	logger.Info(c.Request.Context(), "backup.api.trigger_cleanup",
		zap.String("trace_id", strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))),
	)
	dto.ResponseSuccess(c, gin.H{"status": "success"})
}

func (h *handler) TriggerRestoreDrill(c *gin.Context) {
	var req backupdto.RestoreDrillRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	sourceJobID := parseUint(req.SourceJobID)
	artifactID := parseUint(req.ArtifactID)
	row, err := h.restoreSvc.Trigger(c.Request.Context(), backupops.TriggerRestoreDrillRequest{
		SourceJobID: sourceJobID,
		ArtifactID:  artifactID,
		Reason:      strings.TrimSpace(req.Reason),
		Operator:    resolveOperator(c),
		TraceID:     strings.TrimSpace(reqctx.GetTraceID(c.Request.Context())),
	})
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	logger.Info(c.Request.Context(), "backup.api.trigger_restore_drill",
		zap.Uint64("source_job_id", row.SourceJobID),
		zap.Uint64("drill_id", row.ID),
		zap.String("status", string(row.Status)),
		zap.String("trace_id", strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))),
	)
	dto.ResponseSuccess(c, gin.H{"drill": row})
}

func (h *handler) ListRestoreDrills(c *gin.Context) {
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parseInt(c.DefaultQuery("page_size", "20"), 20)
	sourceJobID := parseUint(c.Query("source_job_id"))
	status := strings.TrimSpace(c.Query("status"))
	from, fromErr := parseDateTime(c.Query("from"))
	if fromErr != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid from datetime", fromErr)
		return
	}
	to, toErr := parseDateTime(c.Query("to"))
	if toErr != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid to datetime", toErr)
		return
	}
	items, total, err := h.restoreSvc.List(c.Request.Context(), backupops.ListRestoreDrillOptions{
		SourceJobID: sourceJobID,
		Status:      status,
		From:        from,
		To:          to,
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items, "pagination": gin.H{"total": total, "page": page, "page_size": pageSize}})
}

func (h *handler) GetRestoreDrill(c *gin.Context) {
	drillID := parseUint(c.Param("drill_id"))
	row, err := h.restoreSvc.Get(c.Request.Context(), drillID)
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"drill": buildDrillDetailResponse(row)})
}

// UpsertPolicy 仅为兼容旧调用方，内部转发到 CreatePolicy。
func (h *handler) UpsertPolicy(c *gin.Context) {
	h.CreatePolicy(c)
}

func (h *handler) ListAlerts(c *gin.Context) {
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parseInt(c.DefaultQuery("page_size", "20"), 20)
	var ackedPtr *bool
	if raw := strings.TrimSpace(strings.ToLower(c.Query("acked"))); raw != "" {
		acked := raw == "true" || raw == "1"
		ackedPtr = &acked
	}
	items, total, err := h.alertSvc.ListAlerts(c.Request.Context(), backupops.ListAlertOptions{
		Level:    strings.TrimSpace(c.Query("level")),
		Acked:    ackedPtr,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": items, "pagination": gin.H{"total": total, "page": page, "page_size": pageSize}})
}

func (h *handler) AckAlert(c *gin.Context) {
	alertID := parseUint(c.Param("alert_id"))
	if err := h.alertSvc.AckAlert(c.Request.Context(), alertID, resolveOperator(c)); err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"alert_id": alertID, "acked": true})
}

func (h *handler) GetBackupOverview(c *gin.Context) {
	overview, err := h.alertSvc.BuildOverview(c.Request.Context())
	if err != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(err))
		return
	}
	dto.ResponseSuccess(c, gin.H{"overview": overview})
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

func parseDateTime(raw string) (*time.Time, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func buildJobDetailResponse(row *modelops.BackupJob) gin.H {
	durationMs := int64(0)
	if row != nil && row.StartedAt != nil && row.EndedAt != nil {
		durationMs = row.EndedAt.Sub(*row.StartedAt).Milliseconds()
	}
	if row == nil {
		return gin.H{}
	}
	return gin.H{
		"id":            row.ID,
		"policy_id":     row.PolicyID,
		"status":        row.Status,
		"trigger_type":  row.TriggerType,
		"started_at":    row.StartedAt,
		"ended_at":      row.EndedAt,
		"duration_ms":   durationMs,
		"trace_id":      row.TraceID,
		"error_summary": row.ErrorMessage,
		"operator":      row.Operator,
	}
}

func buildDrillDetailResponse(row *modelops.RestoreDrillRecord) gin.H {
	durationMs := row.UpdatedAt.Sub(row.CreatedAt).Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}
	return gin.H{
		"id":             row.ID,
		"source_job_id":  row.SourceJobID,
		"status":         row.Status,
		"started_at":     row.CreatedAt,
		"ended_at":       row.UpdatedAt,
		"duration_ms":    durationMs,
		"rto_seconds":    row.RTOSec,
		"result_summary": row.ReportURI,
		"report_uri":     row.ReportURI,
		"trace_id":       row.TraceID,
	}
}
