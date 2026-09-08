package plugin_release

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/service/plugin_release/distribution"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type signingKeyRequest struct {
	KeyID     string `json:"key_id" binding:"required,max=128"`
	PublicKey string `json:"public_key" binding:"required"`
}

type signingKeyResponse struct {
	KeyUUID string `json:"key_uuid"`
	KeyID   string `json:"key_id"`
	Enabled bool   `json:"enabled"`
}

func (h *distributionHandler) registerSigningKey(c *gin.Context) {
	var req signingKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseValidationError(c, err)
		return
	}
	key, err := h.svc.RegisterSigningKey(c.Request.Context(), distribution.RegisterSigningKeyInput{
		KeyID:     strings.TrimSpace(req.KeyID),
		PublicKey: strings.TrimSpace(req.PublicKey),
	})
	if err != nil {
		h.writeSigningKeyError(c, err)
		return
	}
	dto.ResponseSuccessWithStatus(c, http.StatusCreated, signingKeyResponse{
		KeyUUID: key.UUID.String(),
		KeyID:   key.KeyID,
		Enabled: key.Enabled,
	})
}

func (h *distributionHandler) disableSigningKey(c *gin.Context) {
	keyUUID, err := uuid.Parse(strings.TrimSpace(c.Param("key_uuid")))
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid key_uuid", err)
		return
	}
	if err := h.svc.DisableSigningKey(c.Request.Context(), keyUUID); err != nil {
		h.writeSigningKeyError(c, err)
		return
	}
	dto.ResponseSuccess(c, gin.H{"key_uuid": keyUUID.String(), "enabled": false})
}

func (h *distributionHandler) writeSigningKeyError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, distribution.ErrInvalidInput):
		dto.ResponseError(c, http.StatusBadRequest, "invalid signing key", err)
	case errors.Is(err, distribution.ErrSigningKeyNotFound):
		dto.ResponseError(c, http.StatusNotFound, "signing key not found", err)
	default:
		dto.ResponseError(c, http.StatusInternalServerError, "signing key operation failed", err)
	}
}
