package knowledge_space

import (
	"errors"
	querydto "github.com/ArtisanCloud/PowerX/internal/transport/http/knowledge_content"

	svc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func (h *hostContractHandler) contentOverview(c *gin.Context) {
	tenant, err := h.access.AuthorizeContentOverviewRead(c.Request.Context(), reqctx.AuthenticatedAPIKeyHash(c.Request.Context()))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	if h.service == nil || h.service.ContentOverview() == nil {
		dto.RespondErrorFrom(c, svc.KnowledgeUpstreamDependencyError(errors.New("content overview unavailable")))
		return
	}
	query, err := querydto.Query(c.Request.URL.Query(), c.Param("space_uuid"))
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	result, err := h.service.ContentOverview().Read(c.Request.Context(), tenant, query)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	dto.ResponseSuccess(c, result)
}
