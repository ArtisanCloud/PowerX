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

func (h *handler) GetLogConfig(c *gin.Context) {
	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	operator := resolveOperator(c)
	cfg, err := h.svc.GetConfig()
	if err != nil {
		logger.Info(c.Request.Context(), "monitor.logs.config",
			zap.String("operator", operator),
			zap.String("trace_id", traceID),
			zap.String("status", "failed"),
			zap.String("error", err.Error()),
		)
		dto.ResponseError(c, http.StatusInternalServerError, "query monitor log config failed", err)
		return
	}
	logger.Info(c.Request.Context(), "monitor.logs.config",
		zap.String("operator", operator),
		zap.String("trace_id", traceID),
		zap.String("status", "success"),
		zap.String("driver", string(cfg.Driver)),
	)
	dto.ResponseSuccess(c, cfg)
}
