package plugin_release

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

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
	TenantUUID      string            `json:"tenantUuid" binding:"required"`
	PluginID        string            `json:"pluginId" binding:"required"`
	Version         string            `json:"version" binding:"required"`
	Channel         string            `json:"channel" binding:"required"`
	BuildArtifact   string            `json:"buildArtifactUri" binding:"required"`
	CommitHash      string            `json:"commitHash"`
	ReleaseNotes    string            `json:"releaseNotes"`
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

type releaseListQuery struct {
	Page           int    `form:"page,default=1" binding:"min=1"`
	Size           int    `form:"size,default=20" binding:"min=1,max=100"`
	TenantID       string `form:"tenantId"`
	PluginID       string `form:"pluginId"`
	VersionPrefix  string `form:"version"`
	ApprovalStatus string `form:"approvalStatus"`
	GateStatus     string `form:"gateStatus"`
	CreatedBy      string `form:"createdBy"`
}

type releaseListItemResponse struct {
	candidateResponse
	PlanStatus           string `json:"planStatus"`
	OfflinePackageStatus string `json:"offlinePackageStatus"`
	OfflinePackageCount  int64  `json:"offlinePackageCount"`
	CreatedAt            string `json:"createdAt,omitempty"`
	UpdatedAt            string `json:"updatedAt,omitempty"`
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
	tenant := strings.TrimSpace(req.TenantUUID)
	if tenant == "" {
		dto.ResponseError(c, http.StatusBadRequest, "tenantUuid is required", nil)
		return
	}
	channel := strings.TrimSpace(req.Channel)
	if channel == "" {
		dto.ResponseError(c, http.StatusBadRequest, "channel is required", nil)
		return
	}
	// merge channel into labels
	if req.Labels == nil {
		req.Labels = map[string]string{}
	}
	if _, ok := req.Labels["channel"]; !ok {
		req.Labels["channel"] = channel
	}
	commit := strings.TrimSpace(req.CommitHash)
	if commit == "" && req.Metadata != nil {
		if v, ok := req.Metadata["commitHash"]; ok {
			if s, ok := v.(string); ok {
				commit = strings.TrimSpace(s)
			}
		}
	}
	candidate, err := h.pipeline.SubmitCandidate(c.Request.Context(), pipeline.SubmitCandidateInput{
		TenantID:        tenant,
		PluginID:        strings.TrimSpace(req.PluginID),
		Version:         strings.TrimSpace(req.Version),
		BuildArtifact:   strings.TrimSpace(req.BuildArtifact),
		CommitHash:      commit,
		ReleaseNotes:    strings.TrimSpace(req.ReleaseNotes),
		Labels:          req.Labels,
		Actor:           h.actor(c),
		ActorToken:      actorTokenFromContext(c),
		ApprovalContext: strings.TrimSpace(req.ApprovalContext),
	})
	if err != nil {
		writePipelineError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, toCandidateResponse(candidate))
}

func (h *publishHandler) listReleases(c *gin.Context) {
	if h.pipeline == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "pipeline unavailable", nil)
		return
	}
	var query releaseListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	items, total, err := h.pipeline.ListCandidates(c.Request.Context(), pipeline.ListCandidatesInput{
		Page:           query.Page,
		Size:           query.Size,
		TenantID:       strings.TrimSpace(query.TenantID),
		PluginID:       strings.TrimSpace(query.PluginID),
		VersionPrefix:  strings.TrimSpace(query.VersionPrefix),
		ApprovalStatus: strings.TrimSpace(query.ApprovalStatus),
		GateStatus:     strings.TrimSpace(query.GateStatus),
		CreatedBy:      strings.TrimSpace(query.CreatedBy),
	})
	if err != nil {
		writePipelineError(c, err)
		return
	}
	out := make([]releaseListItemResponse, 0, len(items))
	for _, item := range items {
		resp := toCandidateResponse(item.Candidate)
		entry := releaseListItemResponse{
			candidateResponse:    resp,
			PlanStatus:           item.PlanStatus,
			OfflinePackageStatus: item.OfflinePackageStatus,
			OfflinePackageCount:  item.OfflinePackageCount,
		}
		if !item.Candidate.CreatedAt.IsZero() {
			entry.CreatedAt = item.Candidate.CreatedAt.UTC().Format(time.RFC3339)
		}
		if !item.Candidate.UpdatedAt.IsZero() {
			entry.UpdatedAt = item.Candidate.UpdatedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, entry)
	}
	pagination := &dto.PaginationResponse{
		Total:    total,
		Page:     query.Page,
		PageSize: query.Size,
	}
	dto.ResponseList(c, out, pagination)
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

func (h *publishHandler) deleteRelease(c *gin.Context) {
	if h.pipeline == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "pipeline unavailable", nil)
		return
	}
	candidateUUID, err := parseCandidateID(c.Param("candidateId"))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid candidateId", err)
		return
	}
	if err := h.pipeline.DeleteCandidate(c.Request.Context(), candidateUUID, h.actor(c)); err != nil {
		writePipelineError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusNoContent, gin.H{"deleted": true})
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
	if user := c.GetHeader("X-User-Id"); user != "" {
		return sanitizeActor(user)
	}
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if authorization != "" {
		token := trimBearer(authorization)
		if ident, typ := extractIdentityFromJWT(token); ident != "" {
			return sanitizeActor(prefixIdentity(ident, typ, token))
		}
		// 无可读 identity 时使用截断 token 片段，避免落库整串 JWT
		return sanitizeActor(prefixIdentity("", "", token))
	}
	return "system"
}

// sanitizeActor trims and caps actor identifier to fit DB column length.
func sanitizeActor(actor string) string {
	actor = strings.TrimSpace(actor)
	if len(actor) > 128 {
		return actor[:128]
	}
	return actor
}

// trimBearer removes common bearer prefix to avoid storing full JWTs in audit fields.
func trimBearer(value string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return strings.TrimSpace(value[7:])
	}
	return value
}

// actorFromContext builds actor identity using headers and JWT payload.
func actorFromContext(c *gin.Context) string {
	if user := c.GetHeader("X-User-Id"); user != "" {
		return sanitizeActor(user)
	}
	authz := strings.TrimSpace(c.GetHeader("Authorization"))
	if authz != "" {
		token := trimBearer(authz)
		if ident, typ := extractIdentityFromJWT(token); ident != "" {
			return sanitizeActor(prefixIdentity(ident, typ, token))
		}
		return sanitizeActor(prefixIdentity("", "", token))
	}
	return "system"
}

// actorTokenFromContext returns the truncated token fragment for audit purposes.
func actorTokenFromContext(c *gin.Context) string {
	authz := strings.TrimSpace(c.GetHeader("Authorization"))
	if authz == "" {
		return ""
	}
	return truncateToken(trimBearer(authz))
}

// extractIdentityFromJWT attempts to decode base64url JWT payload and pick a readable identity.
func extractIdentityFromJWT(token string) (string, string) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return "", ""
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ""
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return "", ""
	}
	// Prefer readable fields
	for _, key := range []string{"name", "email", "username", "sub", "tid", "uid"} {
		if v, ok := payload[key]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return s, key
			}
		}
	}
	return "", ""
}

// truncateToken returns a short, non-sensitive token fragment for logging/audit.
func truncateToken(token string) string {
	token = strings.TrimSpace(token)
	if len(token) <= 12 {
		return token
	}
	return token[:6] + "..." + token[len(token)-4:]
}

// prefixIdentity builds a readable actor string with type prefix and token fragment fallback.
func prefixIdentity(ident, identType, token string) string {
	ident = strings.TrimSpace(ident)
	identType = strings.TrimSpace(identType)
	if ident != "" {
		if identType == "" {
			identType = "id"
		}
		return identType + ":" + ident
	}
	tokenFrag := truncateToken(token)
	if tokenFrag != "" {
		return "tok:" + tokenFrag
	}
	return "system"
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
