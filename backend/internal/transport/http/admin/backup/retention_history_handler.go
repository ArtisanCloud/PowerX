package backup

import (
	"context"
	"strconv"

	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) RetentionHistory(c *gin.Context) {
	page, size := 1, 20
	filter := "all"
	valid := true
	for k, v := range c.Request.URL.Query() {
		if len(v) != 1 {
			valid = false
			break
		}
		var e error
		switch k {
		case "page":
			page, e = strconv.Atoi(v[0])
		case "page_size":
			size, e = strconv.Atoi(v[0])
		case "coverage":
			filter = v[0]
		default:
			valid = false
		}
		if e != nil {
			valid = false
		}
	}
	if !valid {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_HISTORY_QUERY_INVALID", "BACKUP_HISTORY_QUERY_INVALID", nil))
		return
	}
	out, e := h.jobSvc.RetentionHistory(c.Request.Context(), page, size, filter)
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) RetentionImportRequest(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_HISTORY_QUERY_INVALID", "BACKUP_HISTORY_QUERY_INVALID", nil))
		return
	}
	out, e := h.jobSvc.RetentionImportRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) RetentionImportRequests(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_HISTORY_QUERY_INVALID", "BACKUP_HISTORY_QUERY_INVALID", nil))
		return
	}
	out, e := h.jobSvc.RetentionImportRequests(c.Request.Context())
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, gin.H{"items": out})
}
func (h *handler) ImportRetentionHistory(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "BACKUP_HISTORY_QUERY_INVALID", "BACKUP_HISTORY_QUERY_INVALID", nil))
		return
	}
	var in backupops.RetentionImportInput
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
	out, e := h.jobSvc.ImportRetentionHistory(ctx, in, func(name string, value backupops.RetentionImportReceipt) error { return send(name, value) })
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
