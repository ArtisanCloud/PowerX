package backup

import (
	"context"
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (h *handler) PreviewCleanupResolution(c *gin.Context) {
	id, token, ok := cleanupReviewInput(c)
	if !ok {
		return
	}
	out, e := h.jobSvc.PreviewCleanupResolution(c.Request.Context(), id, token)
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) CleanupResolutionRequests(c *gin.Context) {
	id, token, ok := cleanupReviewInput(c)
	if !ok {
		return
	}
	out, e := h.jobSvc.CleanupResolutionRequests(c.Request.Context(), id, token)
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": out})
}
func (h *handler) CleanupResolutionRequest(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_RESOLUTION_INPUT_INVALID", "BACKUP_RESOLUTION_INPUT_INVALID", nil))
		return
	}
	out, e := h.jobSvc.CleanupResolutionRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) ResolveCleanupRetained(c *gin.Context) {
	id, e := strconv.ParseUint(c.Param("artifact_id"), 10, 64)
	if len(c.Request.URL.Query()) != 0 || e != nil || id == 0 || strconv.FormatUint(id, 10) != c.Param("artifact_id") {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_RESOLUTION_INPUT_INVALID", "BACKUP_RESOLUTION_INPUT_INVALID", nil))
		return
	}
	var in backupops.CleanupResolutionInput
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
	out, e := h.jobSvc.ResolveCleanupRetained(ctx, id, in, func(v backupops.CleanupResolutionEvent) error {
		if v.Receipt != nil {
			return send(v.Name, v.Receipt)
		}
		return send(v.Name, v.Progress)
	})
	if e != nil {
		if !started() {
			dto.RespondErrorFrom(c, backupops.ToAppError(e))
		} else if ctx.Err() == nil {
			_ = send("uncertain", gin.H{"request_id": in.RequestID})
		}
		return
	}
	_ = send("complete", out)
}
