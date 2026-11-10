package storage

import (
	"errors"
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) systemTableError(c *gin.Context, e error) {
	if errors.Is(e, gov.ErrSystemTableNotFound) {
		fail(c, 404, "STORAGE_TABLE_RETENTION_NOT_APPROVED", "该表未开放类型化清理")
		return
	}
	h.stateError(c, e)
}
func (h *handler) systemTablePolicy(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	out, e := h.service.SystemTableRetention(c.Request.Context(), c.Param("table_id"))
	if e != nil {
		h.systemTableError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) saveSystemTablePolicy(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	var in struct {
		ExpectedRevision *int64                          `json:"expected_revision"`
		Confirm          bool                            `json:"confirm"`
		Policy           *gov.SystemTableRetentionPolicy `json:"policy"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	if in.ExpectedRevision == nil || in.Policy == nil || !in.Confirm {
		h.stateError(c, gov.ErrPolicyInvalid)
		return
	}
	out, e := h.service.SaveSystemTableRetention(c.Request.Context(), c.Param("table_id"), *in.ExpectedRevision, *in.Policy)
	if e != nil {
		h.systemTableError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) systemTablePreview(c *gin.Context) { h.systemTableBatch(c, false) }
func (h *handler) systemTableRun(c *gin.Context)     { h.systemTableBatch(c, true) }
func (h *handler) systemTableBatch(c *gin.Context, apply bool) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	var in struct {
		ExpectedRevision *int64 `json:"expected_revision"`
		Confirm          bool   `json:"confirm"`
		RequestID        string `json:"request_id"`
		PreviewID        string `json:"preview_id"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	if in.ExpectedRevision == nil || apply && !in.Confirm {
		h.stateError(c, gov.ErrPolicyInvalid)
		return
	}
	var out gov.SystemTableRetentionRun
	var e error
	if apply {
		out, e = h.service.RunSystemTableRetention(c.Request.Context(), c.Param("table_id"), *in.ExpectedRevision, in.RequestID, in.PreviewID)
	} else {
		out, e = h.service.PreviewSystemTableRetention(c.Request.Context(), c.Param("table_id"), *in.ExpectedRevision)
	}
	if e != nil {
		h.systemTableError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *handler) adoptSystemTableRetention(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	var in gov.SystemTableRetentionAdoption
	if !cleanupBody(c, &in) {
		return
	}
	out, e := h.service.AdoptSystemTableRetention(c.Request.Context(), c.Param("table_id"), in)
	if e != nil {
		h.systemTableError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) changeSystemTableRetention(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	var in gov.SystemTableRetentionChange
	if !cleanupBody(c, &in) {
		return
	}
	out, e := h.service.ChangeSystemTableRetention(c.Request.Context(), c.Param("table_id"), in)
	if e != nil {
		h.systemTableError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) systemTableRetentionRequest(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	out, e := h.service.SystemTableRetentionRequest(c.Request.Context(), c.Param("table_id"), c.Param("request_id"))
	if e != nil {
		h.systemTableError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) systemTableRetentionRunRequest(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	out, e := h.service.SystemTableRetentionRunRequest(c.Request.Context(), c.Param("table_id"), c.Param("request_id"))
	if e != nil {
		h.systemTableError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
