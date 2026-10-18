package backup

import (
	"net/http"

	snapshotops "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	backupdto "github.com/ArtisanCloud/PowerX/internal/transport/http/admin/backup/dto"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) ListSnapshotPolicies(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	out, e := h.snapshots.Overview(c.Request.Context())
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"policies": out.Policies, "default_policy_id": out.DefaultPolicyID})
}

func (h *handler) PutSnapshotPolicy(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	var input backupdto.SnapshotPolicyRequest
	if !bindRestoreJSON(c, &input) {
		return
	}
	p := snapshotops.Policy{Enabled: input.Enabled, IntervalMinutes: input.IntervalMinutes, RetentionDays: input.RetentionDays, RetentionCount: input.RetentionCount}
	out, e := h.snapshots.PutPolicy(c.Request.Context(), c.Param("policy_id"), input.Name, p)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	status := http.StatusOK
	if c.Request.Method == http.MethodPost {
		status = http.StatusCreated
	}
	dto.ResponseSuccessWithStatus(c, status, gin.H{"policy": out})
}

func (h *handler) SetSnapshotPolicyStatus(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	var input backupdto.SnapshotPolicyStatusRequest
	if !bindRestoreJSON(c, &input) {
		return
	}
	if input.Enabled == nil {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "SNAPSHOT_POLICY_STATUS_REQUIRED", "请指定启停状态", nil))
		return
	}
	if input.MakeDefault && *input.Enabled {
		dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "SNAPSHOT_POLICY_SELECTION_INVALID", "选择系统策略不会开启定时，请分别操作", nil))
		return
	}
	var out snapshotops.NamedPolicy
	var e error
	if input.MakeDefault {
		out, e = h.snapshots.SetDefaultPolicy(c.Request.Context(), c.Param("policy_id"))
	} else {
		out, e = h.snapshots.SetPolicyEnabled(c.Request.Context(), c.Param("policy_id"), *input.Enabled)
	}
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, gin.H{"policy": out})
}
