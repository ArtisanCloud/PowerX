package plugin_release

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/distribution"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type offlineImportHandler struct {
	svc    *distribution.Service
	access *hostContractAccess
}

func newOfflineImportHandler(svc *distribution.Service, db *gorm.DB) *offlineImportHandler {
	if svc == nil || db == nil {
		return nil
	}
	return &offlineImportHandler{svc: svc, access: newHostContractAccess(db)}
}

type offlineImportRequest struct {
	TenantUUID      string `json:"tenant_uuid"`
	PackageUUID     string `json:"package_uuid" binding:"required,uuid"`
	LicenseAccepted bool   `json:"license_accepted" binding:"required"`
	DryRun          bool   `json:"dry_run"`
}

func (h *offlineImportHandler) startImport(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, pluginReleaseUpstream(errors.New("distribution service unavailable")))
		return
	}
	if !rejectTenantOverride(c) {
		return
	}
	var req offlineImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondErrorFrom(c, pluginReleaseInvalidArgument(err))
		return
	}
	if strings.TrimSpace(req.TenantUUID) != "" {
		dto.RespondErrorFrom(c, pluginReleaseInvalidArgument(errors.New("tenant_uuid must not be supplied")))
		return
	}
	tenantUUID, err := h.access.authorize(c.Request.Context(), hostAPIKeyHash(c), pluginReleaseImportsManageCapabilityID, pluginReleaseImportsManageAPIKeyScope, "imports", "manage")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	claims := reqctx.GetClaims(c.Request.Context())
	actor := "api_key"
	if claims != nil && strings.TrimSpace(claims.Subject) != "" {
		actor = strings.TrimSpace(claims.Subject)
	}

	job, err := h.svc.StartOfflineImport(c.Request.Context(), distribution.OfflineImportInput{
		TenantUUID:      tenantUUID,
		PackageUUID:     req.PackageUUID,
		DryRun:          req.DryRun,
		LicenseAccepted: req.LicenseAccepted,
		Actor:           actor,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{
		"job_uuid": job.ID,
		"status":   job.Status,
	})
}

func (h *offlineImportHandler) getImport(c *gin.Context) {
	if h.svc == nil {
		dto.RespondErrorFrom(c, pluginReleaseUpstream(errors.New("distribution service unavailable")))
		return
	}
	if !rejectTenantOverride(c) {
		return
	}
	jobID := strings.TrimSpace(c.Param("job_uuid"))
	if jobID == "" {
		dto.RespondErrorFrom(c, pluginReleaseInvalidArgument(errors.New("job_uuid is required")))
		return
	}
	tenantUUID, err := h.access.authorize(c.Request.Context(), hostAPIKeyHash(c), pluginReleaseImportsReadCapabilityID, pluginReleaseImportsReadAPIKeyScope, "imports", "read")
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	job, err := h.svc.GetImportJob(c.Request.Context(), tenantUUID, jobID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	if job == nil {
		dto.RespondErrorFrom(c, pluginReleaseImportJobNotFound(nil))
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"job_uuid":    job.ID,
		"status":      job.Status,
		"completedAt": job.CompletedAt,
	})
}

func (h *offlineImportHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, distribution.ErrFeatureDisabled):
		dto.RespondErrorFrom(c, pluginReleaseForbidden(err))
	case errors.Is(err, distribution.ErrInvalidInput):
		dto.RespondErrorFrom(c, pluginReleaseInvalidArgument(err))
	default:
		dto.RespondErrorFrom(c, pluginReleaseUpstream(err))
	}
}
