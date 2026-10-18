package media

import (
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/ArtisanCloud/PowerX/internal/transport/http/mediaevents"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func (h *Handler) derivativeEvents(c *gin.Context, client *redis.Client) {
	tenant, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.RespondErrorFrom(c, mediasvc.MediaUnauthorizedError(err))
		return
	}
	mediaevents.Serve(c, client, h.svc, tenant, c.Param("uuid"), nil)
}
