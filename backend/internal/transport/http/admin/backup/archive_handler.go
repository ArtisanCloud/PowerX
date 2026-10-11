package backup

import (
	archiveops "github.com/ArtisanCloud/PowerX/internal/service/archive_ops"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *handler) archiveAvailable(c *gin.Context) bool {
	if h.archives == nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(503, "ARCHIVE_SERVICE_UNAVAILABLE", "请重启新版 Core 以加载归档服务", nil))
		return false
	}
	return true
}
func (h *handler) ArchiveOverview(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	out, e := h.archives.Overview(c.Request.Context())
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"overview": out})
}
func (h *handler) SaveArchivePolicy(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var p archiveops.Policy
	if !bindRestoreJSON(c, &p) {
		return
	}
	out, e := h.archives.SavePolicy(c.Request.Context(), p)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"policy": out})
}
func (h *handler) SaveArchiveTarget(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var p archiveops.Target
	if !bindRestoreJSON(c, &p) {
		return
	}
	out, e := h.archives.SaveTarget(c.Request.Context(), p)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"target": out})
}
func emptyArchiveBody(c *gin.Context) bool { var v struct{}; return bindRestoreJSON(c, &v) }
func (h *handler) TestArchiveTarget(c *gin.Context) {
	if !h.archiveAvailable(c) || !emptyArchiveBody(c) {
		return
	}
	if e := h.archives.TestTarget(c.Request.Context(), c.Param("target_id")); e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"verified": true})
}
func (h *handler) RunArchive(c *gin.Context) {
	if !h.archiveAvailable(c) || !emptyArchiveBody(c) {
		return
	}
	out, e := h.archives.Start(c.Request.Context(), c.Param("policy_id"), c.GetHeader("Idempotency-Key"), resolveOperator(c), reqctx.GetTraceID(c.Request.Context()))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{"job": out})
}
func (h *handler) GetArchive(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	out, e := h.archives.Job(c.Request.Context(), c.Param("job_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"job": out})
}
func (h *handler) RetryArchive(c *gin.Context) {
	if !h.archiveAvailable(c) || !emptyArchiveBody(c) {
		return
	}
	out, e := h.archives.Retry(c.Request.Context(), c.Param("job_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{"job": out})
}
func (h *handler) ProtectArchive(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var v struct {
		Protected *bool `json:"protected"`
	}
	if !bindRestoreJSON(c, &v) {
		return
	}
	if v.Protected == nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_REQUEST_INVALID", "必须显式设置 protected", nil))
		return
	}
	out, e := h.archives.Protect(c.Request.Context(), c.Param("job_id"), *v.Protected)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"job": out})
}
func (h *handler) DownloadArchive(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.archiveAvailable(c) {
		return
	}
	path, cleanup, e := h.archives.DownloadFile(c.Request.Context(), c.Param("job_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	defer cleanup()
	c.FileAttachment(path, "powerx-archive-"+c.Param("job_id")+".tar.gz")
}
func (h *handler) RestoreArchive(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var v struct {
		Confirmation     string `json:"confirmation"`
		AllowOlderServer bool   `json:"allow_older_server"`
	}
	if !bindRestoreJSON(c, &v) {
		return
	}
	if v.Confirmation != c.Param("job_id") {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_CONFIRMATION_REQUIRED", "请确认恢复到独立新目录和新数据库", nil))
		return
	}
	out, e := h.archives.StartRestore(c.Request.Context(), c.Param("job_id"), v.AllowOlderServer, resolveOperator(c), reqctx.GetTraceID(c.Request.Context()))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{"restore": out})
}
func (h *handler) CleanupArchives(c *gin.Context) {
	if !h.archiveAvailable(c) || !emptyArchiveBody(c) {
		return
	}
	n, e := h.archives.Cleanup(c.Request.Context())
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"pruned": n})
}
func (h *handler) GetArchiveRestore(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	r, e := h.archives.RestoreDirectory(c.Request.Context(), c.Param("restore_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"restore": r})
}
func (h *handler) DeleteArchiveRestore(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	if len(c.Request.URL.Query()) > 0 || c.Request.ContentLength > 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_REQUEST_INVALID", "删除只接受确认请求头", nil))
		return
	}
	if e := h.archives.RemoveRestore(c.Request.Context(), c.Param("restore_id"), c.GetHeader("X-Restore-Confirmation")); e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"deleted": true})
}
