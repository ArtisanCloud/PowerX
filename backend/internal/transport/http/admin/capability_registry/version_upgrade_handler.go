package capability_registry

import (
	"encoding/json"
	"errors"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/authz"
	"io"
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type versionUpgradeHandler struct {
	service *capservice.VersionUpgradeService
}

func newVersionUpgradeHandler(deps *shared.Deps) *versionUpgradeHandler {
	store, ok := deps.VersionLockStore.(capservice.VersionUpgradeStore)
	if !ok || deps.DB == nil {
		return nil
	}
	return &versionUpgradeHandler{service: capservice.NewVersionUpgradeService(deps.DB, store)}
}
func (h *versionUpgradeHandler) Confirm(c *gin.Context) {
	if authz.IsAPIKeyAuth(c) {
		dto.ResponseError(c, 403, "capability.upgrade_forbidden", nil)
		return
	}
	var body struct {
		CapabilitiesHash string `json:"capabilities_hash"`
		Reason           string `json:"reason"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		dto.ResponseError(c, 400, "capability.upgrade_invalid", err)
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		dto.ResponseError(c, 400, "capability.upgrade_invalid", err)
		return
	}
	result, err := h.service.Confirm(c.Request.Context(), capservice.VersionUpgradeInput{TenantUUID: c.Param("tenant_uuid"), CapabilityID: c.Param("capabilityId"), CapabilitiesHash: body.CapabilitiesHash, Reason: body.Reason})
	if err != nil {
		status := 500
		switch {
		case errors.Is(err, capservice.ErrVersionUpgradeForbidden):
			status = 403
		case errors.Is(err, capservice.ErrVersionUpgradeInvalid):
			status = 400
		case errors.Is(err, capservice.ErrVersionUpgradeHashMismatch):
			status = 409
		case errors.Is(err, gorm.ErrRecordNotFound):
			status = 404
		}
		dto.ResponseError(c, status, "capability.upgrade_failed", err)
		return
	}
	dto.ResponseSuccess(c, result)
}
