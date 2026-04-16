package monitor

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	monitorlogs "github.com/ArtisanCloud/PowerX/internal/service/monitor_logs"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (h *handler) QueryLogs(c *gin.Context) {
	page := parseInt(c.DefaultQuery("page", "1"), 1)
	pageSize := parseInt(c.DefaultQuery("page_size", "50"), 50)
	jobID := parseUint64(c.Query("job_id"))
	policyID := parseUint64(c.Query("policy_id"))
	from, err := parseTimePtr(c.Query("from"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid from datetime", err)
		return
	}
	to, err := parseTimePtr(c.Query("to"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid to datetime", err)
		return
	}

	traceID := strings.TrimSpace(reqctx.GetTraceID(c.Request.Context()))
	queryTraceID := strings.TrimSpace(c.Query("trace_id"))
	driverRaw := strings.TrimSpace(strings.ToLower(c.Query("driver")))
	if driverRaw != "" && driverRaw != string(monitorlogs.DriverFile) && driverRaw != string(monitorlogs.DriverStdio) && driverRaw != string(monitorlogs.DriverLoki) {
		dto.ResponseError(c, http.StatusBadRequest, "invalid driver, must be one of: file|stdio|loki", nil)
		return
	}
	operator := resolveOperator(c)

	req := monitorlogs.QueryRequest{
		TraceID:  queryTraceID,
		JobID:    jobID,
		PolicyID: policyID,
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		From:     from,
		To:       to,
		Page:     page,
		PageSize: pageSize,
	}
	var (
		result   monitorlogs.QueryResult
		queryErr error
	)
	if driverRaw != "" {
		driver := monitorlogs.Driver(driverRaw)
		result, queryErr = h.svc.QueryByDriver(req, driver)
	} else {
		result, queryErr = h.svc.Query(req)
	}
	if queryErr != nil {
		logger.Info(c.Request.Context(), "monitor.logs.query",
			zap.String("operator", operator),
			zap.String("trace_id", traceID),
			zap.String("filter_driver", driverRaw),
			zap.String("filter_trace_id", queryTraceID),
			zap.Uint64("filter_job_id", jobID),
			zap.Uint64("filter_policy_id", policyID),
			zap.String("filter_keyword", strings.TrimSpace(c.Query("keyword"))),
			zap.Int("page", page),
			zap.Int("page_size", pageSize),
			zap.String("status", "failed"),
			zap.String("error", queryErr.Error()),
		)
		dto.ResponseError(c, http.StatusInternalServerError, "query monitor logs failed", queryErr)
		return
	}
	logger.Info(c.Request.Context(), "monitor.logs.query",
		zap.String("operator", operator),
		zap.String("trace_id", traceID),
		zap.String("filter_driver", driverRaw),
		zap.String("filter_trace_id", queryTraceID),
		zap.Uint64("filter_job_id", jobID),
		zap.Uint64("filter_policy_id", policyID),
		zap.String("filter_keyword", strings.TrimSpace(c.Query("keyword"))),
		zap.Int("page", page),
		zap.Int("page_size", pageSize),
		zap.String("driver", string(result.Meta.Driver)),
		zap.Int("result_total", result.Total),
		zap.Bool("degraded", result.Meta.Degraded),
		zap.String("status", "success"),
	)

	dto.ResponseSuccess(c, gin.H{
		"items": result.Items,
		"pagination": gin.H{
			"total":     result.Total,
			"page":      page,
			"page_size": pageSize,
		},
		"query_meta": result.Meta,
	})
}

func parseInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func parseUint64(raw string) uint64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseTimePtr(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return &t, nil
	}
	return nil, strconv.ErrSyntax
}
