package monitor

import (
	"net/http"
	"strings"

	monitorlogs "github.com/ArtisanCloud/PowerX/internal/service/monitor_logs"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type updateRetentionPolicyRequest struct {
	Policy monitorlogs.RetentionPolicy `json:"policy"`
}

func (h *handler) GetRetentionPolicy(c *gin.Context) {
	operator := resolveOperator(c)
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	policy, err := h.svc.GetRetentionPolicy()
	if err != nil {
		logger.Info(c.Request.Context(), "monitor.logs.retention.policy.get",
			zap.String("operator", operator),
			zap.String("trace_id", traceID),
			zap.String("status", "failed"),
			zap.String("error", err.Error()),
		)
		dto.ResponseError(c, http.StatusInternalServerError, "query log retention policy failed", err)
		return
	}
	logger.Info(c.Request.Context(), "monitor.logs.retention.policy.get",
		zap.String("operator", operator),
		zap.String("trace_id", traceID),
		zap.String("status", "success"),
		zap.Bool("enabled", policy.Enabled),
		zap.String("cron", policy.Cron),
	)
	dto.ResponseSuccess(c, gin.H{"policy": policy})
}

func (h *handler) UpdateRetentionPolicy(c *gin.Context) {
	operator := resolveOperator(c)
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	var req updateRetentionPolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request body", err)
		return
	}
	policy, err := h.svc.UpdateRetentionPolicy(c.Request.Context(), req.Policy, operator)
	if err != nil {
		logger.Info(c.Request.Context(), "monitor.logs.retention.policy.update",
			zap.String("operator", operator),
			zap.String("trace_id", traceID),
			zap.String("status", "failed"),
			zap.String("error", err.Error()),
		)
		dto.ResponseError(c, http.StatusBadRequest, "update log retention policy failed", err)
		return
	}
	logger.Info(c.Request.Context(), "monitor.logs.retention.policy.update",
		zap.String("operator", operator),
		zap.String("trace_id", traceID),
		zap.String("status", "success"),
		zap.Bool("enabled", policy.Enabled),
		zap.String("cron", policy.Cron),
	)
	dto.ResponseSuccess(c, gin.H{"policy": policy})
}
