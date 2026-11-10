package agent

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentsvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	capabilitysvc "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// 管理页同步会修改租户目录；不开放给 API Key/STS 或普通成员。
func requireOnlineModelCatalogAdmin(deps *shared.Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		if capabilitysvc.ServiceCredential(c.Request.Context()) || (!reqctx.IsRoot(c.Request.Context()) && !hasAgentCapabilityApprovalAdminRole(c.Request.Context(), deps)) {
			dto.RespondErrorFrom(c, &dto.AppError{HTTPCode: http.StatusForbidden, Code: "AI_MODEL_SYNC_ADMIN_REQUIRED", Message: "AI_MODEL_SYNC_ADMIN_REQUIRED"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (h *AgentSettingHandler) syncOnlineModels(c *gin.Context) {
	if capabilitysvc.ServiceCredential(c.Request.Context()) {
		dto.RespondErrorFrom(c, &dto.AppError{HTTPCode: http.StatusForbidden, Code: "AI_MODEL_SYNC_ADMIN_REQUIRED", Message: "AI_MODEL_SYNC_ADMIN_REQUIRED"})
		return
	}
	tenant, err := requireTenantContext(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	var in agentsvc.OnlineModelSyncRequest
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || decoder.Decode(new(any)) != io.EOF {
		dto.RespondErrorFrom(c, &dto.AppError{HTTPCode: http.StatusBadRequest, Code: "AI_MODEL_SYNC_REQUEST_INVALID", Message: "AI_MODEL_SYNC_REQUEST_INVALID"})
		return
	}
	result, err := h.svc.SyncOnlineModels(c.Request.Context(), tenant.UUID(), in)
	if err != nil {
		dto.ResponseErrorWithDetails(c, dto.StatusCode(err), dto.MessageOf(err), err, dto.DetailsOf(err))
		return
	}
	dto.ResponseSuccess(c, result)
}
