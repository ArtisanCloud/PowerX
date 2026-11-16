package storage

import (
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) databaseMaintenance(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	out, e := h.service.DatabaseMaintenance(c.Request.Context())
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) databaseMaintenancePreview(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	var in gov.DatabaseMaintenancePreview
	if !cleanupBody(c, &in) {
		return
	}
	out, e := h.service.PreviewDatabaseMaintenance(c.Request.Context(), in)
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) databaseMaintenanceStart(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	var in gov.DatabaseMaintenanceRequest
	if !cleanupBody(c, &in) {
		return
	}
	out, e := h.service.StartDatabaseMaintenanceRequest(c.Request.Context(), in)
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) databaseMaintenanceRequest(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	out, e := h.service.DatabaseMaintenanceRun(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) databaseMaintenanceCancel(c *gin.Context) { h.databaseMaintenanceAction(c, false) }
func (h *handler) databaseMaintenanceReview(c *gin.Context) { h.databaseMaintenanceAction(c, true) }
func (h *handler) databaseMaintenanceAction(c *gin.Context, review bool) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	var in struct {
		Confirm   bool   `json:"confirm"`
		PreviewID string `json:"preview_id,omitempty"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	if !in.Confirm {
		h.stateError(c, gov.ErrPolicyInvalid)
		return
	}
	var out gov.DatabaseMaintenanceRun
	var e error
	if review {
		if in.PreviewID != "" {
			h.stateError(c, gov.ErrPolicyInvalid)
			return
		}
		out, e = h.service.ReviewDatabaseMaintenance(c.Request.Context(), c.Param("request_id"))
	} else {
		if in.PreviewID != "" {
			out, e = h.service.CancelDatabaseMaintenance(c.Request.Context(), c.Param("request_id"), in.PreviewID)
		} else {
			out, e = h.service.CancelDatabaseMaintenance(c.Request.Context(), c.Param("request_id"))
		}
	}
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
