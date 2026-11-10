package backup

import (
	snapshot "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) SnapshotPolicyConfiguration(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	out, e := h.snapshots.PolicyConfiguration(c.Request.Context())
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) AdoptSnapshotPolicyConfiguration(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	var in snapshot.PolicyAdoption
	if !strictBackupControlBody(c, &in) {
		return
	}
	out, e := h.snapshots.AdoptPolicyConfiguration(c.Request.Context(), in)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) ChangeSnapshotPolicyConfiguration(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	var in snapshot.PolicyChange
	if !strictBackupControlBody(c, &in) {
		return
	}
	out, e := h.snapshots.ChangePolicyConfiguration(c.Request.Context(), in)
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) SnapshotPolicyConfigurationRequest(c *gin.Context) {
	if !h.snapshotAvailable(c) {
		return
	}
	out, e := h.snapshots.PolicyConfigurationRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
