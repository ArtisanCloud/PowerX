package backup

import (
	"errors"
	"net/http"

	snapshotops "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	backupdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/backup/dto"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) snapshotAvailable(c *gin.Context) bool {
	if h.snapshots == nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(503, "SNAPSHOT_UNAVAILABLE", "系统快照服务未加载", nil))
		return false
	}
	return true
}
func (h *handler) SnapshotOverview(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	out, e := h.snapshots.Overview(c.Request.Context())
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	c.Header("Cache-Control", "no-store")
	dto.ResponseSuccess(c, gin.H{"overview": out})
}
func (h *handler) SaveSnapshotPolicy(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	var p snapshotops.Policy
	if !bindRestoreJSON(c, &p) {
		return
	}
	out, e := h.snapshots.SavePolicy(c.Request.Context(), p)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"policy": out})
}
func (h *handler) CreateSnapshot(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	var input struct {
		PolicyID string `json:"policy_id"`
	}
	if !bindRestoreJSON(c, &input) {
		return
	}
	out, e := h.snapshots.StartForPolicy(c.Request.Context(), c.GetHeader("Idempotency-Key"), input.PolicyID)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	response := gin.H{"snapshot": out}
	if h.coordinator != nil {
		jobs, err := h.coordinator.List()
		if err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
		for _, job := range jobs {
			if job.Kind == "capture" && job.SnapshotID == out.ID {
				token, err := h.coordinator.ProgressToken(job.ID)
				if err != nil {
					dto.RespondErrorFrom(c, err)
					return
				}
				response["coordinator"] = gin.H{"recovery": job, "progress_token": token}
				break
			}
		}
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, response)
}
func (h *handler) GetSnapshot(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	out, e := h.snapshots.Get(c.Request.Context(), c.Param("snapshot_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"snapshot": out})
}

func (h *handler) DeleteSnapshot(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	var input backupdto.SnapshotDeleteRequest
	if err := c.ShouldBindHeader(&input); err != nil || len(c.Request.URL.Query()) != 0 || c.Request.ContentLength != 0 || (input.DeleteProtected != "" && input.DeleteProtected != "false" && input.DeleteProtected != "true") {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "SNAPSHOT_DELETE_INVALID", "删除只接受快照确认及明确的保护确认，不接受路径或其他参数", nil))
		return
	}
	out, err := h.snapshots.Delete(c.Request.Context(), c.Param("snapshot_id"), input.Confirmation, input.DeleteProtected == "true")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	dto.ResponseSuccess(c, gin.H{"deletion": out})
}
func (h *handler) ExportSnapshot(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if !bindRestoreJSON(c, &input) {
		return
	}
	file, cleanup, e := h.snapshots.ExportFile(c.Request.Context(), c.Param("snapshot_id"), input.Password)
	input.Password = ""
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	defer cleanup()
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.FileAttachment(file, "powerx-"+c.Param("snapshot_id")+".pxsnapshot")
}
func (h *handler) ImportSnapshot(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	if len(c.Request.URL.Query()) > 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "SNAPSHOT_QUERY_FORBIDDEN", "导入不接受 query 覆盖", nil))
		return
	}
	const maxImportBody = 8*1024*1024*1024 + 1024*1024
	if c.Request.ContentLength > maxImportBody {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(413, "SNAPSHOT_PACKAGE_TOO_LARGE", "恢复包超过大小上限", nil))
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImportBody)
	if e := c.Request.ParseMultipartForm(8 * 1024 * 1024); e != nil {
		var limit *http.MaxBytesError
		if errors.As(e, &limit) {
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(413, "SNAPSHOT_PACKAGE_TOO_LARGE", "恢复包超过大小上限", nil))
			return
		}
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "SNAPSHOT_IMPORT_INVALID", "无法读取恢复包表单或超出大小上限", nil))
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	form := c.Request.MultipartForm
	if len(form.File) != 1 || len(form.File["file"]) != 1 || len(form.Value) != 1 || len(form.Value["password"]) != 1 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "SNAPSHOT_IMPORT_INVALID", "只允许一个恢复包及其口令", nil))
		return
	}
	if form.File["file"][0].Size > 8*1024*1024*1024 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(413, "SNAPSHOT_PACKAGE_TOO_LARGE", "恢复包超过大小上限", nil))
		return
	}
	file, e := form.File["file"][0].Open()
	if e != nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "SNAPSHOT_IMPORT_INVALID", "无法读取恢复包", nil))
		return
	}
	defer file.Close()
	out, e := h.snapshots.Import(c.Request.Context(), file, form.Value["password"][0])
	form.Value["password"] = nil
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	c.Header("Cache-Control", "no-store")
	dto.ResponseSuccess(c, gin.H{"snapshot": out})
}
func (h *handler) RestoreSnapshot(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	var input struct {
		Confirmation string `json:"confirmation"`
	}
	if !bindRestoreJSON(c, &input) {
		return
	}
	if input.Confirmation != c.Param("snapshot_id") {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "SNAPSHOT_CONFIRMATION_REQUIRED", "请确认恢复到独立数据副本", nil))
		return
	}
	out, e := h.snapshots.StartRestore(c.Request.Context(), c.Param("snapshot_id"), c.GetHeader("Idempotency-Key"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{"restore": out})
}
func (h *handler) GetSnapshotRestore(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	out, e := h.snapshots.GetRestore(c.Request.Context(), c.Param("restore_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"restore": out})
}
func (h *handler) ProtectSnapshot(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	var input struct {
		Protected *bool `json:"protected"`
	}
	if !bindRestoreJSON(c, &input) {
		return
	}
	if input.Protected == nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "SNAPSHOT_PROTECTION_REQUIRED", "请明确设置保护状态", nil))
		return
	}
	out, e := h.snapshots.Protect(c.Request.Context(), c.Param("snapshot_id"), *input.Protected)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"snapshot": out})
}
