package monitor

import (
	"net/http"
	"strings"

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
