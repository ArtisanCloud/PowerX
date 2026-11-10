package integration_gateway

import (
	"encoding/json"
	"io"
	"strconv"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *APIKeyOwnerHandler) GetExplicitGrants(c *gin.Context) {
	tenant, id, err := h.scope(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	result, err := h.service.GetExplicitGrants(c.Request.Context(), tenant, id)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}

func (h *APIKeyOwnerHandler) AppendExplicitGrants(c *gin.Context) {
	tenant, id, err := h.scope(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	var input struct {
		PermissionIDs []uint64 `json:"permission_ids"`
	}
	decoder := json.NewDecoder(io.LimitReader(c.Request.Body, 16385))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("API_KEY_EXPLICIT_GRANT_INVALID", err))
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		dto.RespondErrorFrom(c, dto.NewBadRequest("API_KEY_EXPLICIT_GRANT_INVALID", nil))
		return
	}
	result, err := h.service.AppendExplicitGrants(c.Request.Context(), tenant, id, input.PermissionIDs, strconv.FormatUint(reqctx.GetUserID(c.Request.Context()), 10))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}

func (h *APIKeyOwnerHandler) RevokeExplicitGrant(c *gin.Context) {
	tenant, id, err := h.scope(c)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	permissionID, err := strconv.ParseUint(c.Param("permission_id"), 10, 64)
	if err != nil || permissionID == 0 {
		dto.RespondErrorFrom(c, dto.NewBadRequest("API_KEY_EXPLICIT_GRANT_INVALID", err))
		return
	}
	result, err := h.service.RevokeExplicitGrant(c.Request.Context(), tenant, id, permissionID, strconv.FormatUint(reqctx.GetUserID(c.Request.Context()), 10))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}
