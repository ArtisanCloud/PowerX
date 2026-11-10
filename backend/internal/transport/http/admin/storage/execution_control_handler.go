package storage

import (
	gov "github.com/ArtisanCloud/PowerX/internal/service/storage_governance"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *handler) executionConfiguration(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	out, e := h.service.ExecutionConfiguration(c.Request.Context())
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) changeExecution(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	var in gov.ExecutionChange
	if !cleanupBody(c, &in) {
		return
	}
	out, e := h.service.ChangeExecution(c.Request.Context(), in)
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
func (h *handler) executionRequest(c *gin.Context) {
	if h.service == nil {
		h.stateError(c, gov.ErrStateUnavailable)
		return
	}
	out, e := h.service.ExecutionRequest(c.Request.Context(), c.Param("request_id"))
	if e != nil {
		h.stateError(c, e)
		return
	}
	dto.ResponseSuccess(c, out)
}
