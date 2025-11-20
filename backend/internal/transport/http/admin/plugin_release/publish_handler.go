package plugin_release

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/distribution"
	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/pipeline"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type publishHandler struct {
	pipeline     *pipeline.Service
	distribution *distribution.Service
}

func newPublishHandler(svc *plugin_release.Service) *publishHandler {
	if svc == nil {
		return nil
	}
	return &publishHandler{
		pipeline:     svc.Pipeline(),
		distribution: svc.Distribution(),
	}
}

type publishCreateRequest struct {
	TenantID        string            `json:"tenantId" binding:"required"`
	PluginID        string            `json:"pluginId" binding:"required"`
	Version         string            `json:"version" binding:"required"`
	BuildArtifact   string            `json:"buildArtifactUri" binding:"required"`
	CommitHash      string            `json:"commitHash" binding:"required"`
	ReleaseNotes    string            `json:"releaseNotes" binding:"required"`
	Labels          map[string]string `json:"labels"`
	Metadata        map[string]any    `json:"metadata"`
	ApprovalContext string            `json:"approvalContext"`
}

type publishUpdateRequest struct {
	BuildArtifact string            `json:"buildArtifactUri"`
	ReleaseNotes  string            `json:"releaseNotes"`
	Labels        map[string]string `json:"labels"`
	ApprovalState string            `json:"approvalStatus"`
}

type artifactUploadResponse struct {
	OfflinePackageID uint64 `json:"offlinePackageId"`
	PackageURI       string `json:"packageUri"`
	Status           string `json:"status"`
}

func (h *publishHandler) createRelease(c *gin.Context) {
	if h.pipeline == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "pipeline unavailable", nil)
		return
	}
	var req publishCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	candidate, err := h.pipeline.SubmitCandidate(c.Request.Context(), pipeline.SubmitCandidateInput{
		TenantID:        strings.TrimSpace(req.TenantID),
		PluginID:        strings.TrimSpace(req.PluginID),
		Version:         strings.TrimSpace(req.Version),
		BuildArtifact:   strings.TrimSpace(req.BuildArtifact),
		CommitHash:      strings.TrimSpace(req.CommitHash),
		ReleaseNotes:    strings.TrimSpace(req.ReleaseNotes),
		Labels:          req.Labels,
		Actor:           h.actor(c),
		ApprovalContext: strings.TrimSpace(req.ApprovalContext),
	})
	if err != nil {
		writePipelineError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toCandidateResponse(candidate))
}

func (h *publishHandler) getRelease(c *gin.Context) {
	if h.pipeline == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "pipeline unavailable", nil)
		return
	}
	candidateUUID, err := parseCandidateID(c.Param("candidateId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid candidateId", err)
		return
	}
	candidate, err := h.pipeline.GetCandidate(c.Request.Context(), candidateUUID)
	if err != nil {
		writePipelineError(c, err)
		return
	}
	if candidate == nil {
		dto.ResponseError(c, http.StatusNotFound, "candidate not found", nil)
		return
	}
	dto.ResponseSuccess(c, toCandidateResponse(candidate))
}

func (h *publishHandler) updateRelease(c *gin.Context) {
	if h.pipeline == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "pipeline unavailable", nil)
		return
	}
	var req publishUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	candidateUUID, err := parseCandidateID(c.Param("candidateId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid candidateId", err)
		return
	}
	if strings.TrimSpace(req.BuildArtifact) == "" && strings.TrimSpace(req.ReleaseNotes) == "" && len(req.Labels) == 0 {
		dto.ResponseError(c, http.StatusBadRequest, "no fields to update", nil)
		return
	}
	candidate, err := h.pipeline.UpdateCandidate(c.Request.Context(), pipeline.UpdateCandidateInput{
		CandidateID:   candidateUUID,
		BuildArtifact: strings.TrimSpace(req.BuildArtifact),
		ReleaseNotes:  strings.TrimSpace(req.ReleaseNotes),
		Labels:        req.Labels,
		Actor:         h.actor(c),
	})
	if err != nil {
		writePipelineError(c, err)
		return
	}
	dto.ResponseSuccess(c, toCandidateResponse(candidate))
}

func (h *publishHandler) uploadArtifact(c *gin.Context) {
	if h.distribution == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "distribution unavailable", nil)
		return
	}
	candidateUUID, err := parseCandidateID(c.Param("candidateId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid candidateId", err)
		return
	}
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid multipart payload", err)
		return
	}
	file, _, err := c.Request.FormFile("artifact")
	if err != nil && !errors.Is(err, http.ErrMissingFile) {
		dto.ResponseError(c, http.StatusBadRequest, "artifact read failed", err)
		return
	}
	var artifactContent []byte
	if file != nil {
		defer file.Close()
		buf := bytes.NewBuffer(nil)
		if _, err := io.Copy(buf, file); err != nil {
			dto.ResponseError(c, http.StatusBadRequest, "artifact read failed", err)
			return
		}
		artifactContent = buf.Bytes()
	}
	packageURI := c.PostForm("packageUri")
	checksum := c.PostForm("checksum")
	signature := c.PostForm("signature")
	dependencies, err := parseStringSliceJSON(c.PostForm("dependencies"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid dependencies", err)
		return
	}
	licenseReport, err := parseMapJSON(c.PostForm("licenseReport"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid licenseReport", err)
		return
	}
	if len(licenseReport) == 0 {
		licenseReport = nil
	}

	pkg, err := h.distribution.StoreOfflinePackage(c.Request.Context(), distribution.StoreOfflinePackageInput{
		CandidateID:          candidateUUID,
		PackageURI:           packageURI,
		Content:              artifactContent,
		Checksum:             checksum,
		SignatureFingerprint: signature,
		Dependencies:         dependencies,
		LicenseReport:        licenseReport,
		Actor:                h.actor(c),
	})
	if err != nil {
		writeDistributionError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, artifactUploadResponse{
		OfflinePackageID: pkg.ID,
		PackageURI:       pkg.PackageURI,
		Status:           pkg.Status,
	})
}

func (h *publishHandler) actor(c *gin.Context) string {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if authorization != "" {
		return authorization
	}
	if user := c.GetHeader("X-User-Id"); user != "" {
		return user
	}
	return "admin"
}

func parseCandidateID(value string) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(value))
}

func parseStringSliceJSON(payload string) ([]string, error) {
	if strings.TrimSpace(payload) == "" {
		return nil, nil
	}
	var result []string
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		return nil, err
	}
	return result, nil
}

func parseMapJSON(payload string) (map[string]any, error) {
	if strings.TrimSpace(payload) == "" {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		return nil, err
	}
	return result, nil
}

func writeDistributionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, distribution.ErrFeatureDisabled):
		dto.ResponseError(c, http.StatusServiceUnavailable, err.Error(), err)
	case errors.Is(err, distribution.ErrInvalidInput):
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), err)
	case errors.Is(err, distribution.ErrCandidateNotFound):
		dto.ResponseError(c, http.StatusNotFound, err.Error(), err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, err.Error(), err)
	}
}
