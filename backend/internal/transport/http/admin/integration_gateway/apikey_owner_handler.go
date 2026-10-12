package integration_gateway

import (
	"encoding/json"
	"io"
	"strconv"

	owners "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type APIKeyOwnerHandler struct{ service *owners.APIKeyOwnerService }

func NewAPIKeyOwnerHandler(service *owners.APIKeyOwnerService) *APIKeyOwnerHandler {
	return &APIKeyOwnerHandler{service: service}
}

type setPluginOwnersRequest struct {
	PluginIDs *[]string `json:"plugin_ids"`
}

func (h *APIKeyOwnerHandler) scope(c *gin.Context) (string, uuid.UUID, error) {
	tenant, err := reqctx.RequireTenantUUID(c.Request.Context())
	if err != nil {
		return "", uuid.Nil, dto.NewUnauthorized("tenant context missing", err)
	}
	tenant, err = reqctx.CanonicalTenantUUID(tenant)
	if err != nil {
		return "", uuid.Nil, dto.NewUnauthorized("tenant context invalid", err)
	}
	id, err := uuid.Parse(c.Param("key_id"))
	if err != nil || id == uuid.Nil {
		return "", uuid.Nil, dto.NewBadRequest("invalid key_id", err)
	}
	return tenant, id, nil
}

func (h *APIKeyOwnerHandler) GetPluginOwners(c *gin.Context) {
	tenant, id, err := h.scope(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	result, err := h.service.GetOwners(c.Request.Context(), tenant, id)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}

func (h *APIKeyOwnerHandler) SetPluginOwners(c *gin.Context) {
	tenant, id, err := h.scope(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var input setPluginOwnersRequest
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 16385))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid plugin owner request", err))
		return
	}
	var extra any
	if input.PluginIDs == nil || decoder.Decode(&extra) != io.EOF {
		dto.RespondErrorFrom(c, dto.NewBadRequest("plugin_ids必须是数组；空数组明确解除绑定", nil))
		return
	}
	actor := strconv.FormatUint(reqctx.GetUserID(c.Request.Context()), 10)
	result, err := h.service.SetOwners(c.Request.Context(), tenant, id, *input.PluginIDs, actor)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}
