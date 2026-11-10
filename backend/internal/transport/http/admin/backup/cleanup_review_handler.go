package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"net/http"
	"strconv"
	"time"

	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) ListCleanupReviews(c *gin.Context) {
	for key, values := range c.Request.URL.Query() {
		if (key != "page" && key != "page_size") || len(values) != 1 {
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_REVIEW_QUERY_INVALID", "BACKUP_REVIEW_QUERY_INVALID", nil))
			return
		}
	}
	in := struct {
		Page     int `form:"page" binding:"min=1,max=100000"`
		PageSize int `form:"page_size" binding:"min=1,max=100"`
	}{Page: 1, PageSize: 20}
	if e := c.ShouldBindQuery(&in); e != nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_REVIEW_PAGE_INVALID", "BACKUP_REVIEW_PAGE_INVALID", nil))
		return
	}
	out, e := h.jobSvc.CleanupReviews(c.Request.Context(), in.Page, in.PageSize)
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) InspectCleanupReview(c *gin.Context) {
	id, token, ok := cleanupReviewInput(c)
	if !ok {
		return
	}
	out, e := h.jobSvc.InspectCleanupReview(c.Request.Context(), id, token)
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}

// VerifyCleanupReview streams a request-scoped read. It creates no detached job or retry.
func (h *handler) VerifyCleanupReview(c *gin.Context) {
	id, token, ok := cleanupReviewInput(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	if claims := reqctx.GetClaims(ctx); claims != nil && claims.ExpiresAt != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, claims.ExpiresAt.Time)
		defer cancel()
	}
	send, started := cleanupEventWriter(c)
	out, e := h.jobSvc.VerifyCleanupReview(ctx, id, token, func(v backupops.CleanupVerification) error { return send("progress", v) })
	if e != nil {
		if !started() {
			var app *dto.AppError
			if !errors.As(e, &app) {
				e = dto.NewErrorWithCode(503, "BACKUP_CLEANUP_REVIEW_UNAVAILABLE", "BACKUP_CLEANUP_REVIEW_UNAVAILABLE", nil)
			}
			dto.RespondErrorFrom(c, e)
			return
		}
		if ctx.Err() == nil {
			_ = send("interrupted", gin.H{"artifact_id": strconv.FormatUint(id, 10), "token": token, "state": "interrupted", "content_verified": false, "read_only": true})
		}
		return
	}
	_ = send("complete", out)
}

func cleanupReviewInput(c *gin.Context) (uint64, string, bool) {
	for key, values := range c.Request.URL.Query() {
		if key != "expected_token" || len(values) != 1 {
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_REVIEW_QUERY_INVALID", "BACKUP_REVIEW_QUERY_INVALID", nil))
			return 0, "", false
		}
	}
	id, e := strconv.ParseUint(c.Param("artifact_id"), 10, 64)
	if e != nil || id == 0 || strconv.FormatUint(id, 10) != c.Param("artifact_id") {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_REVIEW_ARTIFACT_INVALID", "BACKUP_REVIEW_ARTIFACT_INVALID", nil))
		return 0, "", false
	}
	return id, c.Query("expected_token"), true
}

func cleanupEventWriter(c *gin.Context) (func(string, any) error, func() bool) {
	sent := false
	controller := http.NewResponseController(c.Writer)
	send := func(name string, value any) error {
		raw, e := json.Marshal(value)
		if e != nil {
			return e
		}
		if !sent {
			c.Header("Content-Type", "text/event-stream")
			c.Header("Cache-Control", "no-store")
			c.Header("X-Accel-Buffering", "no")
			c.Header("X-Content-Type-Options", "nosniff")
			sent = true
		}
		_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
		defer controller.SetWriteDeadline(time.Time{})
		if _, e = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", name, raw); e != nil {
			return e
		}
		return controller.Flush()
	}
	return send, func() bool { return sent }
}

func (h *handler) RetainCleanupReview(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_RETAIN_INPUT_INVALID", "BACKUP_RETAIN_INPUT_INVALID", nil))
		return
	}
	id, e := strconv.ParseUint(c.Param("artifact_id"), 10, 64)
	if e != nil || id == 0 || strconv.FormatUint(id, 10) != c.Param("artifact_id") {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_REVIEW_ARTIFACT_INVALID", "BACKUP_REVIEW_ARTIFACT_INVALID", nil))
		return
	}
	var in backupops.CleanupRetainInput
	if !strictBackupControlBody(c, &in) {
		return
	}
	ctx := c.Request.Context()
	if claims := reqctx.GetClaims(ctx); claims != nil && claims.ExpiresAt != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, claims.ExpiresAt.Time)
		defer cancel()
	}
	send, started := cleanupEventWriter(c)
	out, e := h.jobSvc.RetainCleanupReview(ctx, id, in, func(event backupops.CleanupRetainEvent) error {
		if event.Receipt != nil {
			return send(event.Name, event.Receipt)
		}
		return send(event.Name, event.Progress)
	})
	if e != nil {
		if !started() {
			var app *dto.AppError
			if !errors.As(e, &app) {
				e = dto.NewErrorWithCode(503, "BACKUP_RETAIN_RESULT_UNKNOWN", "BACKUP_RETAIN_RESULT_UNKNOWN", nil)
			}
			dto.RespondErrorFrom(c, e)
		} else if ctx.Err() == nil {
			_ = send("uncertain", gin.H{"request_id": in.RequestID})
		}
		return
	}
	_ = send("complete", out)
}
func (h *handler) CleanupRetentionRequest(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_RETAIN_INPUT_INVALID", "BACKUP_RETAIN_INPUT_INVALID", nil))
		return
	}
	out, e := h.jobSvc.RetentionRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) CleanupRetentionRequests(c *gin.Context) {
	id, token, ok := cleanupReviewInput(c)
	if !ok {
		return
	}
	out, e := h.jobSvc.RetentionRequests(c.Request.Context(), id, token)
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": out})
}
