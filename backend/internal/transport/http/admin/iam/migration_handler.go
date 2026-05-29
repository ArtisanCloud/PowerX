package iam

import (
	iamsvc "github.com/ArtisanCloud/PowerX/internal/service/iam"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type MigrationHandler struct {
	svc *iamsvc.IAMMigrationReportService
}

func NewMigrationHandler(svc *iamsvc.IAMMigrationReportService) *MigrationHandler {
	return &MigrationHandler{svc: svc}
}

func (h *MigrationHandler) Report(c *gin.Context) {
	out, err := h.svc.Report(c.Request.Context())
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}

func (h *MigrationHandler) FixOwner(c *gin.Context) {
	out, err := h.svc.FixMissingOwners(c.Request.Context())
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, out)
}
