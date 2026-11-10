package integration_gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	owners "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type batchExplicitGrantRequest struct {
	KeyIDs           []uuid.UUID `json:"key_ids"`
	PermissionIDs    []uint64    `json:"permission_ids"`
	DryRun           bool        `json:"dry_run"`
	ExpectedPlanHash string      `json:"expected_plan_hash,omitempty"`
}

func (h *APIKeyOwnerHandler) BatchAppendExplicitGrants(c *gin.Context) {
	tenant, err := reqctx.RequireTenantUUID(c.Request.Context())
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("缺少有效租户上下文", err))
		return
	}
	var input batchExplicitGrantRequest
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		respondAPIKeyPermissionError(c, dto.WithCode(dto.NewBadRequest("批量授权参数无效", err), "API_KEY_BATCH_GRANT_INVALID"))
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		respondAPIKeyPermissionError(c, dto.WithCode(dto.NewBadRequest("批量授权参数无效", nil), "API_KEY_BATCH_GRANT_INVALID"))
		return
	}
	result, err := h.service.BatchAppendExplicitGrants(c.Request.Context(), tenant, owners.BatchExplicitGrantInput{KeyIDs: input.KeyIDs, PermissionIDs: input.PermissionIDs, DryRun: input.DryRun, ExpectedPlanHash: input.ExpectedPlanHash}, strconv.FormatUint(reqctx.GetUserID(c.Request.Context()), 10))
	if err != nil {
		respondAPIKeyPermissionError(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}
