package root

import (
	"strconv"

	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type SupportSessionHandler struct {
	svc *iamsvc.RootSupportSessionService
}

type StartSupportSessionRequest struct {
	TargetTenantUUID string `json:"target_tenant_uuid" validate:"required"`
	Reason           string `json:"reason" validate:"required"`
	Mode             string `json:"mode" validate:"omitempty,oneof=read_only write_enabled"`
}

func NewSupportSessionHandler(svc *iamsvc.RootSupportSessionService) *SupportSessionHandler {
	return &SupportSessionHandler{svc: svc}
}

func (h *SupportSessionHandler) Start(c *gin.Context) {
	var req StartSupportSessionRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("参数错误", err))
		return
	}
	out, err := h.svc.Start(c.Request.Context(), iamsvc.StartRootSupportSessionInput{
		TargetTenantUUID: req.TargetTenantUUID,
		Reason:           req.Reason,
		Mode:             req.Mode,
	})
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *SupportSessionHandler) End(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		dto.RespondErrorFrom(c, dto.NewBadRequest("support session id invalid", err))
		return
	}
	out, err := h.svc.End(c.Request.Context(), id)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
