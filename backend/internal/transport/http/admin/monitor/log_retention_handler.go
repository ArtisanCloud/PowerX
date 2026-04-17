package monitor

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *handler) ListRetentionRuns(c *gin.Context) {
	operator := resolveOperator(c)
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	limit := parseInt(c.DefaultQuery("limit", "20"), 20)
	if limit > 100 {
		limit = 100
	}
	data := h.svc.RetentionRuns(limit)
	logger.Info(c.Request.Context(), "monitor.logs.retention.runs",
		zap.String("operator", operator),
		zap.String("trace_id", traceID),
		zap.Int("limit", limit),
		zap.Int("result_count", len(data.Items)),
		zap.String("status", "success"),
	)
	dto.ResponseSuccess(c, data)
}

func (h *handler) TriggerRetentionRun(c *gin.Context) {
	operator := resolveOperator(c)
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	run, err := h.svc.TriggerRetentionNow(c.Request.Context(), operator)
	if err != nil {
		logger.Info(c.Request.Context(), "monitor.logs.retention.run",
			zap.String("operator", operator),
			zap.String("trace_id", traceID),
			zap.String("status", "failed"),
			zap.String("error", err.Error()),
		)
		dto.ResponseError(c, http.StatusInternalServerError, "trigger log retention failed", err)
		return
	}
	logger.Info(c.Request.Context(), "monitor.logs.retention.run",
		zap.String("operator", operator),
		zap.String("trace_id", traceID),
		zap.String("run_id", run.RunID),
		zap.String("status", run.Status),
		zap.Int64("deleted_files", run.DeletedFiles),
		zap.Int64("deleted_rows", run.DeletedRows),
	)
	dto.ResponseSuccess(c, gin.H{"run": run})
}

func (h *handler) TriggerRetentionDryRun(c *gin.Context) {
	operator := resolveOperator(c)
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	var retentionDays *int
	if raw := strings.TrimSpace(c.Query("retention_days")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 0 {
			dto.ResponseError(c, http.StatusBadRequest, "invalid retention_days", nil)
			return
		}
		retentionDays = &parsed
	}
	run, err := h.svc.TriggerRetentionDryRun(c.Request.Context(), operator, retentionDays)
	if err != nil {
		logger.Info(c.Request.Context(), "monitor.logs.retention.dry_run",
			zap.String("operator", operator),
			zap.String("trace_id", traceID),
			zap.String("status", "failed"),
			zap.String("error", err.Error()),
		)
		dto.ResponseError(c, http.StatusInternalServerError, "trigger log retention dry-run failed", err)
		return
	}
	logger.Info(c.Request.Context(), "monitor.logs.retention.dry_run",
		zap.String("operator", operator),
		zap.String("trace_id", traceID),
		zap.String("run_id", run.RunID),
		zap.Int("retention_days", run.RetentionDays),
		zap.String("cutoff_at", run.CutoffAt.Format(time.RFC3339)),
		zap.String("status", run.Status),
		zap.Int64("matched_files", run.DeletedFiles),
		zap.Int64("matched_rows", run.DeletedRows),
	)
	dto.ResponseSuccess(c, gin.H{"run": run})
}

func (h *handler) ExportRetentionDryRun(c *gin.Context) {
	operator := resolveOperator(c)
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	var retentionDays *int
	if raw := strings.TrimSpace(c.Query("retention_days")); raw != "" {
		parsed, parseErr := strconv.Atoi(raw)
		if parseErr != nil || parsed < 0 {
			dto.ResponseError(c, http.StatusBadRequest, "invalid retention_days", nil)
			return
		}
		retentionDays = &parsed
	}

	var cutoffAt *time.Time
	if raw := strings.TrimSpace(c.Query("cutoff_at")); raw != "" {
		parsed, parseErr := time.Parse(time.RFC3339, raw)
		if parseErr != nil {
			dto.ResponseError(c, http.StatusBadRequest, "invalid cutoff_at, expect RFC3339", nil)
			return
		}
		cutoffAt = &parsed
	}

	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "txt")))
	if format != "txt" && format != "json" {
		dto.ResponseError(c, http.StatusBadRequest, "invalid format, support txt/json", nil)
		return
	}

	exported, err := h.svc.ExportRetentionDryRun(c.Request.Context(), operator, retentionDays, cutoffAt, format)
	if err != nil {
		logger.Info(c.Request.Context(), "monitor.logs.retention.export",
			zap.String("operator", operator),
			zap.String("trace_id", traceID),
			zap.String("status", "failed"),
			zap.String("error", err.Error()),
		)
		dto.ResponseError(c, http.StatusInternalServerError, "export retention dry-run failed", err)
		return
	}
	logger.Info(c.Request.Context(), "monitor.logs.retention.export",
		zap.String("operator", operator),
		zap.String("trace_id", traceID),
		zap.String("run_id", exported.RunID),
		zap.Int("retention_days", exported.RetentionDays),
		zap.String("cutoff_at", exported.CutoffAt.Format(time.RFC3339)),
		zap.Int64("matched_files", exported.MatchedFiles),
		zap.Int64("matched_rows", exported.MatchedRows),
		zap.String("format", exported.Format),
		zap.Int("file_size", exported.File.SizeBytes),
		zap.String("status", "success"),
	)
	dto.ResponseSuccess(c, gin.H{"export": exported})
}
