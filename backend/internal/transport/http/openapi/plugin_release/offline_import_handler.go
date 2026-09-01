package plugin_release

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/distribution"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type offlineImportHandler struct {
	svc *distribution.Service
}

func newOfflineImportHandler(svc *distribution.Service) *offlineImportHandler {
	if svc == nil {
		return nil
	}
	return &offlineImportHandler{svc: svc}
}

type offlineImportRequest struct {
	PackageURI      string `json:"packageUri" binding:"required"`
	Checksum        string `json:"checksum" binding:"required"`
	LicenseAccepted bool   `json:"licenseAccepted" binding:"required"`
	DryRun          bool   `json:"dryRun"`
}

func (h *offlineImportHandler) startImport(c *gin.Context) {
	if h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "distribution service unavailable", nil)
		return
	}
	actor := strings.TrimSpace(c.GetHeader("Authorization"))
	if actor == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "authorization header required", nil)
		return
	}
	var req offlineImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "PLUGIN_RELEASE_UNAUTHORIZED", err)
		return
	}

	job, err := h.svc.StartOfflineImport(c.Request.Context(), distribution.OfflineImportInput{
		TenantUUID:      tenantUUID,
		PackageURI:      req.PackageURI,
		Checksum:        req.Checksum,
		DryRun:          req.DryRun,
		LicenseAccepted: req.LicenseAccepted,
		Actor:           actor,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusAccepted, gin.H{
		"jobId":  job.ID,
		"status": job.Status,
	})
}

func (h *offlineImportHandler) getImport(c *gin.Context) {
	if h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "distribution service unavailable", nil)
		return
	}
	actor := strings.TrimSpace(c.GetHeader("Authorization"))
	if actor == "" {
		dto.ResponseError(c, http.StatusUnauthorized, "authorization header required", nil)
		return
	}
	jobID := strings.TrimSpace(c.Param("jobId"))
	if jobID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "jobId is required", nil)
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusUnauthorized, "PLUGIN_RELEASE_UNAUTHORIZED", err)
		return
	}
	job, err := h.svc.GetImportJob(c.Request.Context(), tenantUUID, jobID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	if job == nil {
		dto.ResponseError(c, http.StatusNotFound, "PLUGIN_RELEASE_IMPORT_NOT_FOUND", nil)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"jobId":       job.ID,
		"status":      job.Status,
		"completedAt": job.CompletedAt,
		"actor":       actor,
	})
}

func (h *offlineImportHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, distribution.ErrFeatureDisabled):
		dto.ResponseError(c, http.StatusForbidden, "PLUGIN_RELEASE_FORBIDDEN", err)
	case errors.Is(err, distribution.ErrInvalidInput):
		dto.ResponseError(c, http.StatusBadRequest, "PLUGIN_RELEASE_INVALID_ARGUMENT", err)
	default:
		dto.ResponseError(c, http.StatusServiceUnavailable, "PLUGIN_RELEASE_UPSTREAM_DEPENDENCY", err)
	}
}
