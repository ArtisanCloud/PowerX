package storage

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/infra/opswatch"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/eventstream"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"path/filepath"

	budget "github.com/ArtisanCloud/PowerX/internal/infra/storage_budget"
	lifecycle "github.com/ArtisanCloud/PowerX/internal/infra/storage_lifecycle"
	storagegov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *handler) runtimeStatus(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	out, err := h.service.Runtime(c.Request.Context())
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) saveRuntimePolicy(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	var in struct {
		ExpectedRevision *int64 `json:"expected_revision"`
		Enabled          *bool  `json:"enabled"`
		RetentionDays    int    `json:"retention_days"`
		MaxBytes         int64  `json:"max_bytes"`
		RequireArchive   *bool  `json:"require_archive"`
		CleanupMode      string `json:"cleanup_mode"`
		Confirm          bool   `json:"confirm"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	if !in.Confirm || in.ExpectedRevision == nil || in.Enabled == nil || in.RequireArchive == nil || *in.ExpectedRevision < 0 {
		fail(c, 400, "STORAGE_RUNTIME_CONFIRM_REQUIRED", "请核对策略与归档要求并明确确认")
		return
	}
	out, err := h.service.SaveRuntimePolicy(c.Request.Context(), c.Param("source_id"), *in.ExpectedRevision, lifecycle.Policy{CleanupMode: in.CleanupMode, Enabled: *in.Enabled, RetentionDays: in.RetentionDays, MaxBytes: in.MaxBytes, RequireArchive: *in.RequireArchive})
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) saveRuntimeBudget(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	var in struct {
		ExpectedRevision  *int64 `json:"expected_revision"`
		MinFreeBytes      int64  `json:"min_free_bytes"`
		CriticalFreeBytes int64  `json:"critical_free_bytes"`
		MaxUsedBytes      int64  `json:"max_used_bytes"`
		MaxConcurrent     int    `json:"max_concurrent"`
		Confirm           bool   `json:"confirm"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	if !in.Confirm || in.ExpectedRevision == nil || *in.ExpectedRevision < 1 {
		fail(c, 400, "STORAGE_RUNTIME_CONFIRM_REQUIRED", "请核对主机共享预算并确认")
		return
	}
	out, err := h.service.SaveRuntimeBudget(c.Request.Context(), *in.ExpectedRevision, budget.BudgetPolicy{MinFreeBytes: in.MinFreeBytes, CriticalFreeBytes: in.CriticalFreeBytes, MaxUsedBytes: in.MaxUsedBytes, MaxConcurrent: in.MaxConcurrent})
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) runtimePreview(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	var in struct {
		SourceID      string `json:"source_id"`
		RetentionDays int    `json:"retention_days"`
		MaxBytes      int64  `json:"max_bytes"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	out, err := h.service.PreviewRuntimePolicy(c.Request.Context(), in.SourceID, in.RetentionDays, in.MaxBytes)
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) runtimeRun(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	var in struct {
		SourceID         string `json:"source_id"`
		RequestID        string `json:"request_id"`
		Confirm          bool   `json:"confirm"`
		ExpectedRevision int64  `json:"expected_revision"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	if _, err := uuid.Parse(in.RequestID); err != nil || !in.Confirm || in.ExpectedRevision < 1 {
		fail(c, 400, "STORAGE_RUNTIME_CONFIRM_REQUIRED", "请使用预览版本确认本轮执行")
		return
	}
	out, err := h.service.RunRuntimeRequest(c.Request.Context(), in.SourceID, in.RequestID, in.ExpectedRevision)
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) runtimeEvents(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		fail(c, 400, "STORAGE_EVENTS_QUERY_FORBIDDEN", "状态推送不接受 query 参数")
		return
	}
	ctx := c.Request.Context()
	if claims := reqctx.GetClaims(ctx); claims != nil && claims.ExpiresAt != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, claims.ExpiresAt.Time)
		defer cancel()
	}
	sub, err := opswatch.Subscribe(ctx, map[string][]string{"storage": h.service.RuntimeChangeDirectories()}, func(path string) bool {
		name := filepath.Base(path)
		return name == "budget.json" || name == "lifecycle.json" || name == "alerts.json" || name == "database-retention.json"
	})
	if err != nil {
		fail(c, 503, "STORAGE_EVENTS_UNAVAILABLE", "存储状态推送暂不可用，请手动刷新")
		return
	}
	defer sub.Close()
	_ = eventstream.Serve(ctx, c.Writer, sub, func(names []string) (eventstream.Event, error) {
		h.service.PublishChange("runtime_file_changed")
		name := "change"
		if names == nil {
			name = "snapshot"
		}
		return eventstream.Event{Name: name, Data: gin.H{"resources": []string{"storage"}}}, nil
	})
}
func (h *handler) runtimeAcknowledge(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	var in struct {
		Confirm bool `json:"confirm"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	if !in.Confirm {
		fail(c, 400, "STORAGE_RUNTIME_CONFIRM_REQUIRED", "请先暂停来源并核对未确认记录")
		return
	}
	out, err := h.service.AcknowledgeRuntime(c.Request.Context(), c.Param("run_id"))
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
