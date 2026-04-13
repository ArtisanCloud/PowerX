package monitor

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	monitorlogs "github.com/ArtisanCloud/PowerX/internal/service/monitor_logs"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
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

	result, err := h.svc.Query(monitorlogs.QueryRequest{
		TraceID:  strings.TrimSpace(c.Query("trace_id")),
		JobID:    jobID,
		PolicyID: policyID,
		Keyword:  strings.TrimSpace(c.Query("keyword")),
		From:     from,
		To:       to,
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		dto.ResponseError(c, http.StatusInternalServerError, "query monitor logs failed", err)
		return
	}

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
