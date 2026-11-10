package backup

import (
	"context"
	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"strconv"
)

func quarantineContext(c *gin.Context) (context.Context, context.CancelFunc) {
	ctx := c.Request.Context()
	if claims := reqctx.GetClaims(ctx); claims != nil && claims.ExpiresAt != nil {
		return context.WithDeadline(ctx, claims.ExpiresAt.Time)
	}
	return ctx, func() {}
}
func (h *handler) PreviewCleanupQuarantine(c *gin.Context) {
	id, token, ok := cleanupReviewInput(c)
	if !ok {
		return
	}
	ctx, cancel := quarantineContext(c)
	defer cancel()
	send, started := cleanupEventWriter(c)
	out, e := h.jobSvc.PreviewCleanupQuarantine(ctx, id, token, func(v backupops.CleanupVerification) error { return send("progress", v) })
	if e != nil {
		if !started() {
			dto.RespondErrorFrom(c, backupops.ToAppError(e))
		} else if ctx.Err() == nil {
			_ = send("incomplete", gin.H{"artifact_id": c.Param("artifact_id")})
		}
		return
	}
	_ = send("complete", out)
}
func (h *handler) ResolveCleanupQuarantine(c *gin.Context) {
	id, e := strconv.ParseUint(c.Param("artifact_id"), 10, 64)
	if len(c.Request.URL.Query()) != 0 || e != nil || id == 0 || strconv.FormatUint(id, 10) != c.Param("artifact_id") {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_QUARANTINE_INPUT_INVALID", "BACKUP_QUARANTINE_INPUT_INVALID", nil))
		return
	}
	var in backupops.CleanupQuarantineInput
	if !strictBackupControlBody(c, &in) {
		return
	}
	ctx, cancel := quarantineContext(c)
	defer cancel()
	send, started := cleanupEventWriter(c)
	out, e := h.jobSvc.ResolveCleanupQuarantine(ctx, id, in, func(v backupops.CleanupResolutionEvent) error {
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
func (h *handler) CleanupQuarantineRequest(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_QUARANTINE_INPUT_INVALID", "BACKUP_QUARANTINE_INPUT_INVALID", nil))
		return
	}
	out, e := h.jobSvc.CleanupQuarantineRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) CleanupQuarantineRequests(c *gin.Context) {
	id, token, ok := cleanupReviewInput(c)
	if !ok {
		return
	}
	out, e := h.jobSvc.CleanupQuarantineRequests(c.Request.Context(), id, token)
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": out})
}
