package plugin_release

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/distribution"
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
	TenantID        string `json:"tenantId"`
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
	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" {
		tenantID = strings.TrimSpace(c.GetHeader("X-Tenant-ID"))
	}
	if tenantID == "" {
		dto.ResponseError(c, http.StatusBadRequest, "tenantId is required", nil)
		return
	}

	job, err := h.svc.StartOfflineImport(c.Request.Context(), distribution.OfflineImportInput{
		TenantID:        tenantID,
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
	job, err := h.svc.GetImportJob(jobID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	if job == nil {
		dto.ResponseError(c, http.StatusNotFound, "import job not found", nil)
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
		dto.ResponseError(c, http.StatusForbidden, "offline distribution disabled", err)
	case errors.Is(err, distribution.ErrInvalidInput):
		dto.ResponseError(c, http.StatusBadRequest, "invalid offline import request", err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "offline import operation failed", err)
	}
}
