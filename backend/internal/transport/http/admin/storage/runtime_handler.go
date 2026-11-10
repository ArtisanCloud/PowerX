package storage

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/infra/opswatch"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/eventstream"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"path/filepath"

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
		RequestID        string `json:"request_id"`
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
	if !in.Confirm || in.RequestID == "" || in.ExpectedRevision == nil || in.Enabled == nil || in.RequireArchive == nil || *in.ExpectedRevision < 1 || in.CleanupMode == "" || *in.RequireArchive != (in.CleanupMode == "archive_then_delete") {
		fail(c, 400, "STORAGE_RUNTIME_CONFIRM_REQUIRED", "请核对策略与归档要求并明确确认")
		return
	}
	out, err := h.service.SaveLifecycleConfiguration(c.Request.Context(), c.Param("source_id"), *in.ExpectedRevision, in.RequestID, storagegov.LifecycleSettings{CleanupMode: in.CleanupMode, Enabled: *in.Enabled, RetentionDays: in.RetentionDays, MaxBytes: in.MaxBytes})
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
	var in storagegov.HostBudgetChange
	if !cleanupBody(c, &in) {
		return
	}
	out, err := h.service.SaveHostBudget(c.Request.Context(), in)
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
		return name == "budget.json" || name == "lifecycle.json" || name == "alerts.json" || name == "database-retention.json" || name == "table-monitoring-control.json"
	})
	if err != nil {
		fail(c, 503, "STORAGE_EVENTS_UNAVAILABLE", "存储状态推送暂不可用，请手动刷新")
		return
	}
	run, cancel := context.WithCancel(ctx)
	updates, unsubscribe := h.service.SubscribeRuntime()
	notifications := make(chan []string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(notifications)
		for {
			select {
			case <-run.Done():
				return
			case _, ok := <-sub.Changes:
				if !ok {
					return
				}
				h.service.PublishChange("runtime_file_changed")
			case <-updates:
				select {
				case notifications <- []string{"storage"}:
				default:
				}
			}
		}
	}()
	defer func() { cancel(); <-done; unsubscribe(); sub.Close() }()
	_ = eventstream.ServeNotifications(run, c.Writer, notifications, sub.Errors, func(names []string) (eventstream.Event, error) {
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

func (h *handler) budgetConfiguration(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	out, err := h.service.HostBudgetConfiguration(c.Request.Context())
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) adoptBudget(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	var in storagegov.HostBudgetAdoption
	if !cleanupBody(c, &in) {
		return
	}
	out, err := h.service.AdoptHostBudget(c.Request.Context(), in)
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) budgetRequest(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, storagegov.ErrStateUnavailable)
		return
	}
	out, err := h.service.HostBudgetRequest(c.Request.Context(), c.Param("request_id"))
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
