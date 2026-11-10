package backup

import (
	archive "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) ReviewArchiveInterruption(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_REVIEW_INPUT_INVALID", "此核验不接受查询参数", nil))
		return
	}
	send, sent := cleanupEventWriter(c)
	out, e := h.archives.ReviewInterruptedCapture(c.Request.Context(), c.Param("job_id"), func(p archive.InterruptionProgress) error { return send("progress", p) })
	if e != nil {
		if !sent() {
			dto.RespondErrorFrom(c, e)
		} else {
			_ = send("error", gin.H{"problem": dto.CodeOf(e)})
		}
		return
	}
	_ = send("complete", out)
}
func (h *handler) RetainArchiveInterruption(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var in archive.RetainInterruptedInput
	if !bindRestoreJSON(c, &in) {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_DISPOSITION_INPUT_INVALID", "不接受查询参数", nil))
		return
	}
	out, e := h.archives.RetainInterruptedCapture(c.Request.Context(), c.Param("job_id"), in)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) ArchiveInterruptionRequest(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.archiveAvailable(c) {
		return
	}
	out, e := h.archives.InterruptionRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *handler) ReviewArchiveInterruptionRequest(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.archiveAvailable(c) {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_REVIEW_INPUT_INVALID", "不接受查询参数", nil))
		return
	}
	out, e := h.archives.ReviewInterruptionRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) CloseArchiveInterruptionRequest(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.archiveAvailable(c) {
		return
	}
	var in archive.CloseInterruptionRequestInput
	if !bindRestoreJSON(c, &in) {
		return
	}
	if len(c.Request.URL.Query()) != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_CLOSURE_INPUT_INVALID", "不接受查询参数", nil))
		return
	}
	out, e := h.archives.CloseInterruptionRequest(c.Request.Context(), c.Param("request_id"), in)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
