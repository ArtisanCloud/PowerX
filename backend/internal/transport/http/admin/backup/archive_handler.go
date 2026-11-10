package backup

import (
	"encoding/json"
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
	var p archiveops.PolicyChange
	if !bindRestoreJSON(c, &p) {
		return
	}
	out, e := h.archives.SavePolicyConfiguration(c.Request.Context(), p)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) SaveArchiveTarget(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var p archiveops.TargetChange
	if !bindRestoreJSON(c, &p) {
		return
	}
	out, e := h.archives.SaveTargetConfiguration(c.Request.Context(), p)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
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
	if !h.archiveAvailable(c) {
		return
	}
	var input struct {
		ExpectedPolicyRevision json.RawMessage `json:"expected_policy_revision"`
	}
	if !bindRestoreJSON(c, &input) {
		return
	}
	var out archiveops.Job
	var e error
	if len(input.ExpectedPolicyRevision) == 0 {
		out, e = h.archives.Start(c.Request.Context(), c.Param("policy_id"), c.GetHeader("Idempotency-Key"), resolveOperator(c), reqctx.GetTraceID(c.Request.Context()))
	} else {
		var revision int64
		if json.Unmarshal(input.ExpectedPolicyRevision, &revision) != nil || revision < 1 {
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_POLICY_REVISION_REQUIRED", "策略版本必须是正整数", nil))
			return
		}
		out, e = h.archives.StartAtRevision(c.Request.Context(), c.Param("policy_id"), c.GetHeader("Idempotency-Key"), revision, resolveOperator(c), reqctx.GetTraceID(c.Request.Context()))
	}
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{"job": out})
}

func (h *handler) ArchiveRunRequest(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !h.archiveAvailable(c) {
		return
	}
	if len(c.Request.URL.Query()) != 0 || c.Request.ContentLength != 0 {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_REQUEST_INVALID", "原请求核对仅接受路径中的策略和请求编号", nil))
		return
	}
	out, e := h.archives.RunRequest(c.Request.Context(), c.Param("policy_id"), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) CreateOneTimeArchive(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var input struct {
		SourceID string `json:"source_id"`
	}
	if !bindRestoreJSON(c, &input) {
		return
	}
	out, e := h.archives.StartOneTime(c.Request.Context(), input.SourceID, c.GetHeader("Idempotency-Key"), resolveOperator(c), reqctx.GetTraceID(c.Request.Context()))
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

func (h *handler) MigrateArchive(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var in struct {
		PolicyID string `json:"policy_id"`
		Confirm  bool   `json:"confirm"`
	}
	if !bindRestoreJSON(c, &in) {
		return
	}
	if !in.Confirm {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_CONFIRM_REQUIRED", "请确认只复制并保留旧对象", nil))
		return
	}
	out, err := h.archives.MigrateArchive(c.Request.Context(), c.Param("job_id"), in.PolicyID, c.GetHeader("Idempotency-Key"), resolveOperator(c))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, 202, gin.H{"job": out})
}

func (h *handler) ReconcileArchiveRemoteGC(c *gin.Context) {
	if !h.archiveAvailable(c) || !emptyArchiveBody(c) {
		return
	}
	out, err := h.archives.ReconcileRemoteGC(c.Request.Context(), c.Param("preview_id"))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *handler) ArchiveTargetConfiguration(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	out, e := h.archives.TargetConfiguration(c.Request.Context())
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) AdoptArchiveTargetConfiguration(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var in struct {
		archiveops.TargetAdoption
		Confirm bool `json:"confirm"`
	}
	if !bindRestoreJSON(c, &in) {
		return
	}
	if !in.Confirm {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_CONFIRMATION_REQUIRED", "需要明确确认接管", nil))
		return
	}
	out, e := h.archives.AdoptTargetConfiguration(c.Request.Context(), in.TargetAdoption)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) ArchiveTargetConfigurationRequest(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	out, e := h.archives.TargetConfigurationRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *handler) ArchivePolicyConfiguration(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	out, e := h.archives.PolicyConfiguration(c.Request.Context())
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) AdoptArchivePolicyConfiguration(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	var in struct {
		archiveops.PolicyAdoption
		Confirm bool `json:"confirm"`
	}
	if !bindRestoreJSON(c, &in) {
		return
	}
	if !in.Confirm {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "ARCHIVE_CONFIRMATION_REQUIRED", "请明确确认暂停并接管", nil))
		return
	}
	out, e := h.archives.AdoptPolicyConfiguration(c.Request.Context(), in.PolicyAdoption)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) ArchivePolicyConfigurationRequest(c *gin.Context) {
	if !h.archiveAvailable(c) {
		return
	}
	out, e := h.archives.PolicyConfigurationRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
