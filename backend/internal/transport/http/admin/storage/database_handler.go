package storage

import (
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) databasePolicy(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	out, err := h.service.DatabaseRetention(c.Request.Context())
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) saveDatabasePolicy(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	var in struct {
		ExpectedRevision *int64                       `json:"expected_revision"`
		Policy           *gov.DatabaseRetentionPolicy `json:"policy"`
		Confirm          bool                         `json:"confirm"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	if !in.Confirm || in.Policy == nil || in.ExpectedRevision == nil {
		h.stateError(c, gov.ErrPolicyInvalid)
		return
	}
	out, err := h.service.SaveDatabaseRetention(c.Request.Context(), *in.ExpectedRevision, *in.Policy)
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) databasePreview(c *gin.Context) { h.databaseBatch(c, false) }
func (h *handler) databaseRun(c *gin.Context)     { h.databaseBatch(c, true) }
func (h *handler) databaseBatch(c *gin.Context, apply bool) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	var in struct {
		ExpectedRevision *int64 `json:"expected_revision"`
		RequestID        string `json:"request_id"`
		Confirm          bool   `json:"confirm"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	if in.ExpectedRevision == nil {
		h.stateError(c, gov.ErrPolicyInvalid)
		return
	}
	var out gov.DatabaseRetentionRun
	var err error
	if !apply {
		out, err = h.service.PreviewDatabaseRetention(c.Request.Context(), *in.ExpectedRevision)
	} else if in.Confirm {
		out, err = h.service.RunDatabaseRetention(c.Request.Context(), *in.ExpectedRevision, in.RequestID)
	} else {
		h.stateError(c, gov.ErrPolicyInvalid)
		return
	}
	if err != nil {
		h.stateError(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
