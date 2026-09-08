package media

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	mediasvc "github.com/ArtisanCloud/PowerX/internal/service/media"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

// Register mounts only the UUID-only tenant Media Host Contract.
func Register(publicGroup *gin.RouterGroup, protectedGroup *gin.RouterGroup, deps *shared.Deps) {
	_ = publicGroup
	if deps == nil || deps.MediaSvc == nil || protectedGroup == nil {
		return
	}
	registerHostContract(protectedGroup, deps)
}

// RegisterPublicResource mounts only signed transfer endpoints. Asset and
// variant resources are never anonymously addressable by a legacy UUID/name
// route; callers must first obtain a scoped ticket from the tenant Host API.
func RegisterPublicResource(engine *gin.Engine, deps *shared.Deps) {
	if engine == nil || deps == nil || deps.MediaSvc == nil {
		return
	}
	engine.GET("/media/transfers/:asset_uuid/download", serveHostTransfer(deps.MediaSvc, "download"))
	engine.PUT("/media/transfers/:asset_uuid/upload", serveHostTransfer(deps.MediaSvc, "upload"))
}

func serveHostTransfer(svc *mediasvc.MediaService, action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		asset, err := svc.AuthorizeHostTransfer(c.Request.Context(), c.Param("asset_uuid"), action, c.Query("exp"), c.Query("version"), c.Query("ticket"))
		if err != nil {
			dto.RespondErrorFrom(c, hostMediaError(err))
			return
		}
		if action == "upload" {
			size, err := strconv.ParseInt(strings.TrimSpace(c.GetHeader("Content-Length")), 10, 64)
			if err != nil {
				dto.RespondErrorFrom(c, mediasvc.MediaUploadValidationFailedError(err))
				return
			}
			if err := svc.PutHostTransfer(c.Request.Context(), asset, c.Request.Body, size, c.GetHeader("Content-Type")); err != nil {
				dto.RespondErrorFrom(c, hostMediaError(err))
				return
			}
			c.Status(http.StatusNoContent)
			return
		}
		opened, object, err := svc.OpenAssetResource(c.Request.Context(), asset.TenantUUID, asset.UUID)
		if err != nil || object == nil {
			dto.RespondErrorFrom(c, hostMediaError(err))
			return
		}
		defer object.Body.Close()
		c.DataFromReader(http.StatusOK, object.Size, object.ContentType, object.Body, nil)
		_ = opened
	}
}
