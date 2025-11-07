package plugin_release

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/distribution"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type distributionHandler struct {
	svc *distribution.Service
}

func newDistributionHandler(svc *distribution.Service) *distributionHandler {
	if svc == nil {
		return nil
	}
	return &distributionHandler{svc: svc}
}

type createOfflinePackageRequest struct {
	ReleaseCandidateID   string         `json:"releaseCandidateId" binding:"required"`
	PackageURI           string         `json:"packageUri"`
	Checksum             string         `json:"checksum" binding:"required"`
	SignatureFingerprint string         `json:"signatureFingerprint"`
	Dependencies         []string       `json:"dependencies"`
	LicenseReport        map[string]any `json:"licenseReport"`
}

func (h *distributionHandler) createOfflinePackage(c *gin.Context) {
	if h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "distribution service unavailable", nil)
		return
	}
	var req createOfflinePackageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	candidateUUID, err := uuid.Parse(strings.TrimSpace(req.ReleaseCandidateID))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid releaseCandidateId", err)
		return
	}
	pkg, err := h.svc.StoreOfflinePackage(c.Request.Context(), distribution.StoreOfflinePackageInput{
		CandidateID:          candidateUUID,
		PackageURI:           strings.TrimSpace(req.PackageURI),
		Checksum:             req.Checksum,
		SignatureFingerprint: req.SignatureFingerprint,
		Dependencies:         req.Dependencies,
		LicenseReport:        req.LicenseReport,
		Actor:                c.GetHeader("Authorization"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{
		"id":                 pkg.ID,
		"releaseCandidateId": req.ReleaseCandidateID,
		"packageUri":         pkg.PackageURI,
		"status":             pkg.Status,
		"checksum":           pkg.Checksum,
	})
}

type createListingRequest struct {
	OfflinePackageID uint64         `json:"offlinePackageId" binding:"required"`
	Channel          string         `json:"channel" binding:"required"`
	Pricing          map[string]any `json:"pricing"`
	SupportPolicy    map[string]any `json:"supportPolicy"`
	SubmissionForm   map[string]any `json:"submissionForm"`
}

func (h *distributionHandler) createListing(c *gin.Context) {
	if h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "distribution service unavailable", nil)
		return
	}
	var req createListingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	listing, err := h.svc.SubmitListing(c.Request.Context(), distribution.SubmitListingInput{
		OfflinePackageID: req.OfflinePackageID,
		Channel:          req.Channel,
		Pricing:          req.Pricing,
		SupportPolicy:    req.SupportPolicy,
		SubmissionForm:   req.SubmissionForm,
		Actor:            c.GetHeader("Authorization"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, gin.H{
		"id":               listing.ID,
		"offlinePackageId": listing.OfflinePackageID,
		"channel":          listing.Channel,
		"reviewStatus":     listing.ReviewStatus,
		"reviewCount":      listing.ReviewCount,
		"escalatedAt":      listing.EscalatedAt,
	})
}

type reviewListingRequest struct {
	Decision string `json:"decision" binding:"required"`
	Comment  string `json:"comment"`
}

func (h *distributionHandler) reviewListing(c *gin.Context) {
	if h.svc == nil {
		dto.ResponseError(c, http.StatusServiceUnavailable, "distribution service unavailable", nil)
		return
	}
	listingID, err := strconv.ParseUint(strings.TrimSpace(c.Param("listingId")), 10, 64)
	if err != nil || listingID == 0 {
		dto.ResponseError(c, http.StatusBadRequest, "invalid listingId", err)
		return
	}
	var req reviewListingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	listing, err := h.svc.ReviewListing(c.Request.Context(), distribution.ReviewListingInput{
		ListingID: listingID,
		Decision:  req.Decision,
		Comment:   req.Comment,
		Actor:     c.GetHeader("Authorization"),
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{
		"id":           listing.ID,
		"reviewStatus": listing.ReviewStatus,
		"reviewCount":  listing.ReviewCount,
		"escalatedAt":  listing.EscalatedAt,
	})
}

func (h *distributionHandler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, distribution.ErrFeatureDisabled):
		dto.ResponseError(c, http.StatusForbidden, "offline distribution disabled", err)
	case errors.Is(err, distribution.ErrInvalidInput):
		dto.ResponseError(c, http.StatusBadRequest, "invalid distribution request", err)
	case errors.Is(err, distribution.ErrCandidateNotFound),
		errors.Is(err, distribution.ErrPackageNotFound),
		errors.Is(err, distribution.ErrListingNotFound):
		dto.ResponseError(c, http.StatusNotFound, err.Error(), err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "distribution operation failed", err)
	}
}
