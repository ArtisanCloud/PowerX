package backup

import (
	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) ReviewArchiveCommand(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.archiveAvailable(c) {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_REVIEW_INPUT_INVALID", "不接受查询参数", nil))
		return
	}
	out, e := h.archives.ReviewCommandDrain(c.Request.Context(), c.Param("record_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) ConfirmArchiveCommand(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.archiveAvailable(c) {
		return
	}
	var in archive.ConfirmCommandDrainInput
	if !bindRestoreJSON(c, &in) {
		return
	}
	out, e := h.archives.ConfirmCommandDrain(c.Request.Context(), c.Param("record_id"), in)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) ArchiveCommandConfirmationRequest(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.archiveAvailable(c) {
		return
	}
	out, e := h.archives.CommandConfirmationRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
