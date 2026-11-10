package storage

import (
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) lifecycleConfiguration(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	out, e := h.service.LifecycleConfiguration(c.Request.Context(), c.Param("source_id"))
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) adoptLifecycle(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	var in struct {
		RequestID      string `json:"request_id"`
		LegacyRevision *int64 `json:"legacy_revision"`
		LegacyDigest   string `json:"legacy_digest"`
		Confirm        bool   `json:"confirm"`
	}
	if !cleanupBody(c, &in) {
		return
	}
	if !in.Confirm || in.LegacyRevision == nil || *in.LegacyRevision < 0 || len(in.LegacyDigest) != 64 {
		h.stateError(c, gov.ErrPolicyInvalid)
		return
	}
	out, e := h.service.AdoptLifecycle(c.Request.Context(), c.Param("source_id"), *in.LegacyRevision, in.LegacyDigest, in.RequestID)
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) lifecycleRequest(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	out, e := h.service.LifecycleRequest(c.Request.Context(), c.Param("source_id"), c.Param("request_id"))
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
