package capability_registry

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	capservice "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/admin/authz"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
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
		respondVersionUpgradeError(c, capservice.ErrVersionUpgradeForbidden, 0, "capability.upgrade_forbidden")
		return
	}
	var body struct {
		CapabilitiesHash string `json:"capabilities_hash"`
		Reason           string `json:"reason"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		respondVersionUpgradeError(c, capservice.ErrVersionUpgradeInvalid, 0, "capability.upgrade_invalid")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		respondVersionUpgradeError(c, capservice.ErrVersionUpgradeInvalid, 0, "capability.upgrade_invalid")
		return
	}
	result, err := h.service.Confirm(c.Request.Context(), capservice.VersionUpgradeInput{TenantUUID: c.Param("tenant_uuid"), CapabilityID: c.Param("capabilityId"), CapabilitiesHash: body.CapabilitiesHash, Reason: body.Reason})
	if err != nil {
		respondVersionUpgradeError(c, err, result.AuditID, "capability.upgrade_failed")
		return
	}
	dto.ResponseSuccess(c, result)
}

// respondVersionUpgradeError 发布固定机器码；诊断错误不作为客户端判断依据。
func respondVersionUpgradeError(c *gin.Context, err error, auditID uint64, message string) {
	status, code, retryable := 500, "capability.upgrade_failed", false
	switch {
	case errors.Is(err, capservice.ErrVersionUpgradeUnavailable):
		status, code, retryable = 503, "capability.upgrade_unavailable", true
	case errors.Is(err, capservice.ErrVersionUpgradePersistenceFailed):
		status, code, retryable = 503, "capability.upgrade_persistence_failed", true
	case errors.Is(err, capservice.ErrVersionUpgradeRuntimeApplyFailed):
		status, code, retryable = 503, "capability.upgrade_runtime_apply_failed", true
	case errors.Is(err, capservice.ErrVersionUpgradeAuditFinalizeFailed):
		status, code, retryable = 500, "capability.upgrade_audit_finalize_failed", true
	case errors.Is(err, capservice.ErrVersionUpgradeForbidden):
		status, code = 403, "capability.upgrade_forbidden"
	case errors.Is(err, capservice.ErrVersionUpgradeInvalid):
		status, code = 400, "capability.upgrade_invalid"
	case errors.Is(err, capservice.ErrVersionUpgradeHashMismatch):
		status, code = 409, "capability.upgrade_hash_mismatch"
	case errors.Is(err, capservice.ErrVersionUpgradeBindingMismatch):
		status, code = 409, "capability.upgrade_binding_mismatch"
	case errors.Is(err, capservice.ErrVersionUpgradeNotFound):
		status, code = 404, "capability.upgrade_not_found"
	}
	details := map[string]any{"reason_code": code, "retryable": retryable}
	if trace := reqctx.GetTraceID(c.Request.Context()); trace != "" {
		details["trace_id"] = trace
	}
	if auditID > 0 {
		details["audit_id"] = auditID
		details["approval_persisted"] = true
	}
	if code == "capability.upgrade_audit_finalize_failed" {
		details["runtime_confirmed"] = true
	}
	// ResponseError 会输出 err.Error()，所以仅向统一桥接传递公开、安全的业务错误。
	public := &dto.AppError{HTTPCode: status, Code: code, Message: code}
	dto.ResponseErrorWithDetails(c, status, message, public, details)
}
