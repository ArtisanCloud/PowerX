package backup

import (
	"bytes"
	"encoding/json"
	"io"

	backupops "github.com/ArtisanCloud/PowerX/internal/service/backup_ops"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func strictBackupControlBody(c *gin.Context, in any) bool {
	raw, e := io.ReadAll(io.LimitReader(c.Request.Body, 32769))
	if e != nil || len(raw) > 32768 {
		dto.ResponseError(c, 400, "BACKUP_POLICY_BODY_INVALID", nil)
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(in) != nil || d.Decode(new(any)) != io.EOF {
		dto.ResponseError(c, 400, "BACKUP_POLICY_BODY_INVALID", nil)
		return false
	}
	return true
}
func (h *handler) BackupPolicyConfiguration(c *gin.Context) {
	out, e := h.policySvc.ControlConfiguration(c.Request.Context())
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) AdoptBackupPolicyConfiguration(c *gin.Context) {
	var in backupops.PolicyAdoption
	if !strictBackupControlBody(c, &in) {
		return
	}
	out, e := h.policySvc.AdoptConfiguration(c.Request.Context(), in)
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) ChangeBackupPolicyConfiguration(c *gin.Context) {
	var in backupops.PolicyChange
	if !strictBackupControlBody(c, &in) {
		return
	}
	out, e := h.policySvc.ChangeConfiguration(c.Request.Context(), in)
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) BackupPolicyConfigurationRequest(c *gin.Context) {
	out, e := h.policySvc.ConfigurationRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		dto.RespondErrorFrom(c, backupops.ToAppError(e))
		return
	}
	dto.ResponseSuccess(c, out)
}
