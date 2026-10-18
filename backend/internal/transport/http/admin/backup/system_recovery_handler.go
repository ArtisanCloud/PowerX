package backup

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"net/http"
)

func (h *handler) recoveryAvailable(c *gin.Context) bool {
	if h.coordinator == nil || !h.coordinator.Online() {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(503, "RECOVERY_COORDINATOR_OFFLINE", "独立恢复协调器未配置或未在线", nil))
		return false
	}
	return true
}
func (h *handler) RecoveryOverview(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if h.coordinator == nil {
		dto.ResponseSuccess(c, gin.H{"configured": false, "online": false, "targets": []any{}, "jobs": []any{}, "generation": 0})
		return
	}
	jobs, e := h.coordinator.List()
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	tokens := map[string]string{}
	for _, job := range jobs {
		token, e := h.coordinator.ProgressToken(job.ID)
		if e != nil {
			dto.RespondErrorFrom(c, e)
			return
		}
		tokens[job.ID] = token
	}
	state, e := h.coordinator.EnvironmentState()
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	state["configured"] = true
	state["online"] = h.coordinator.Online()
	state["targets"] = h.coordinator.Targets
	state["reason"] = h.coordinator.RestoreProblem
	state["jobs"] = jobs
	state["progress_tokens"] = tokens
	if h.coordinator.RecoveryEntry != "" && h.coordinator.PrepareSession != nil {
		if err := h.coordinator.PrepareSession(c.Writer, c.Request, fmt.Sprintf("user:%d", reqctx.GetUserID(c.Request.Context()))); err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
		state["recovery_entry"] = h.coordinator.RecoveryEntry
	}
	dto.ResponseSuccess(c, state)
}
func (h *handler) PrepareSystemRecovery(c *gin.Context) {
	if !h.recoveryAvailable(c) {
		return
	}
	var input struct {
		Confirmation string `json:"confirmation"`
		TargetID     string `json:"target_id"`
		RestoreMode  string `json:"restore_mode"`
		Generation   uint64 `json:"generation"`
	}
	if !bindRestoreJSON(c, &input) {
		return
	}
	if input.Confirmation != c.Param("snapshot_id") {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "RECOVERY_CONFIRMATION_REQUIRED", "请确认所选快照和目标环境", nil))
		return
	}
	if input.TargetID != "" || input.RestoreMode != "" {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "RECOVERY_REQUEST_CHANGED", "恢复仅替换当前实例，不再接受目标环境或切换模式", nil))
		return
	}
	if h.snapshots != nil {
		snapshot, e := h.snapshots.Get(c.Request.Context(), input.Confirmation)
		if e != nil {
			dto.RespondErrorFrom(c, e)
			return
		}
		allowed := snapshot.RecoverySupported
		for _, target := range h.coordinator.Targets {
			if target.AllowOverwrite && target.Environment == "dev" && target.AllowOnlineData {
				allowed = true
			}
		}
		if snapshot.Status != "ready" || !allowed {
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(409, "RECOVERY_PACKAGE_UNSUPPORTED", "此包尚未就绪或缺少整站恢复合同；在线包只允许显式本地开发演练", nil))
			return
		}
	}
	// 页面可长时间保持打开；提交时刷新维护会话，避免业务停下后才发现旧 Cookie 已过期。
	if h.coordinator.PrepareSession != nil {
		if err := h.coordinator.PrepareSession(c.Writer, c.Request, fmt.Sprintf("user:%d", reqctx.GetUserID(c.Request.Context()))); err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
	}
	job, e := h.coordinator.EnqueueInstanceRestore(c.Request.Context(), input.Confirmation, c.GetHeader("Idempotency-Key"), input.Generation)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	token, e := h.coordinator.ProgressToken(job.ID)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{"recovery": job, "progress_token": token})
}
func (h *handler) CutoverSystemRecovery(c *gin.Context) {
	dto.RespondErrorFrom(c, dto.NewErrorWithCode(410, "RECOVERY_SWITCH_REMOVED", "环境切换已停用，请选择快照恢复当前实例", nil))
}
func (h *handler) RollbackSystemRecovery(c *gin.Context) { h.environmentAction(c, "rollback") }
func (h *handler) CleanupSystemRecovery(c *gin.Context)  { h.environmentAction(c, "cleanup") }
func (h *handler) environmentAction(c *gin.Context, action string) {
	dto.RespondErrorFrom(c, dto.NewErrorWithCode(410, "RECOVERY_SWITCH_REMOVED", "环境管理已停用；需要回退时请选择恢复前保护快照重新恢复", nil))
}
