package plugin_release

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/runtime"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type deploymentHandler struct {
	runtime *runtime.Service
}

func newDeploymentHandler(rt *runtime.Service) *deploymentHandler {
	if rt == nil {
		return nil
	}
	return &deploymentHandler{runtime: rt}
}

type triggerCanaryRequest struct {
	BatchName string `json:"batchName" binding:"required"`
}

func (h *deploymentHandler) triggerCanary(c *gin.Context) {
	if h.runtime == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "runtime service unavailable", nil)
		return
	}
	planID, err := parsePlanID(c.Param("planId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid planId", err)
		return
	}
	var req triggerCanaryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	events, err := h.runtime.TriggerCanary(c.Request.Context(), runtime.TriggerCanaryInput{
		PlanID:    planID,
		BatchName: strings.TrimSpace(req.BatchName),
		Actor:     c.GetHeader("Authorization"),
	})
	if err != nil {
		writeRuntimeError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"planId": planID,
		"events": events,
	})
}

type finalizeRequest struct {
	Action string `json:"action" binding:"required"`
}

func (h *deploymentHandler) finalizeDeployment(c *gin.Context) {
	if h.runtime == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "runtime service unavailable", nil)
		return
	}
	planID, err := parsePlanID(c.Param("planId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid planId", err)
		return
	}
	var req finalizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	plan, err := h.runtime.FinalizeDeployment(c.Request.Context(), runtime.FinalizeInput{
		PlanID: planID,
		Action: req.Action,
		Actor:  c.GetHeader("Authorization"),
	})
	if err != nil {
		writeRuntimeError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"planId": plan.ID,
		"status": plan.Status,
	})
}

func (h *deploymentHandler) rollback(c *gin.Context) {
	if h.runtime == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "runtime service unavailable", nil)
		return
	}
	planID, err := parsePlanID(c.Param("planId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid planId", err)
		return
	}
	plan, err := h.runtime.FinalizeDeployment(c.Request.Context(), runtime.FinalizeInput{
		PlanID: planID,
		Action: "rollback",
		Actor:  c.GetHeader("Authorization"),
	})
	if err != nil {
		writeRuntimeError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"planId": plan.ID,
		"status": plan.Status,
	})
}

func parsePlanID(raw string) (uint64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, errors.New("planId is required")
	}
	return strconv.ParseUint(value, 10, 64)
}

func writeRuntimeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, runtime.ErrInvalidInput):
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
	case errors.Is(err, runtime.ErrPlanNotFound):
		dto.ResponseError(c, http.StatusNotFound, err.Error(), err)
	case errors.Is(err, runtime.ErrBatchNotFound):
		dto.ResponseError(c, http.StatusNotFound, err.Error(), err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "runtime operation failed", err)
	}
}
