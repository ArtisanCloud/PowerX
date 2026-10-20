package media

import (
	"github.com/ArtisanCloud/PowerX/internal/transport/http/mediaevents"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Events are optional; original upload/reuse needs only the existing Media grants.
func (h *hostContractHandler) derivativeEvents(c *gin.Context, client *redis.Client) {
	if err := h.svc.AuthorizeImageDerivativeRead(c.Request.Context()); err != nil {
		dto.RespondErrorFrom(c, err)
		return
	}
	mediaevents.Serve(c, client, h.svc, reqctx.GetTenantUUID(c.Request.Context()), c.Param("asset_uuid"), h.svc.AuthorizeImageDerivativeRead)
}
