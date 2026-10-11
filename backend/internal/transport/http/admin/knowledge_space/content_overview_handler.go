package knowledge_space

import (
	"errors"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	svc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	querydto "github.com/ArtisanCloud/PowerX/internal/transport/http/knowledge_content"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func registerContentOverviewAdmin(protected *gin.RouterGroup, deps *shared.Deps) {
	if deps.KnowledgeSpace == nil || deps.KnowledgeSpace.ContentOverview == nil {
		return
	}
	protected.GET("/admin/knowledge-spaces/:spaceId/content-overview", func(c *gin.Context) {
		claims := reqctx.GetClaims(c.Request.Context())
		if claims == nil || claims.UserID == 0 || claims.Issuer == "powerx-sts" || reqctx.AuthenticatedAPIKeyHash(c.Request.Context()) != "" {
			dto.RespondErrorFrom(c, svc.KnowledgeUnauthorizedError(errors.New("admin user required")))
			return
		}
		tenant, err := reqctx.RequireTenantUUID(c.Request.Context())
		if err != nil {
			dto.RespondErrorFrom(c, svc.KnowledgeUnauthorizedError(err))
			return
		}
		input, err := querydto.Query(c.Request.URL.Query(), c.Param("spaceId"))
		if err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
		result, err := deps.KnowledgeSpace.ContentOverview.Read(c.Request.Context(), tenant, input)
		if err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
		dto.ResponseSuccess(c, result)
	})
}
