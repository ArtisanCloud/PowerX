package plugin_identity

import (
	"io"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	svc "github.com/ArtisanCloud/PowerX/internal/service/plugin_identity"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

func Register(group *gin.RouterGroup, deps *shared.Deps) {
	if group == nil || deps == nil || deps.PluginRequestIdentity == nil {
		return
	}
	group.POST("/admin/iam/identity-contexts:resolve", func(c *gin.Context) {
		operation := c.Param("resolve")
		if (operation != ":resolve" && operation != ":revalidate-ws") || len(c.Request.URL.Query()) > 0 {
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "IAM_CONTEXT_INVALID_ARGUMENT", "IAM_CONTEXT_INVALID_ARGUMENT", nil))
			return
		}
		raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 4097))
		if err != nil || len(raw) > 4096 {
			dto.RespondErrorFrom(c, dto.NewErrorWithCode(400, "IAM_CONTEXT_INVALID_ARGUMENT", "IAM_CONTEXT_INVALID_ARGUMENT", nil))
			return
		}
		var result svc.Result
		if operation == ":resolve" {
			var request svc.Query
			if err = svc.Decode(raw, &request); err == nil {
				result, err = deps.PluginRequestIdentity.ResolveAdmin(c.Request.Context(), request)
			}
		} else {
			var request svc.WSQuery
			if err = svc.Decode(raw, &request); err == nil {
				result, err = deps.PluginRequestIdentity.RevalidateWSAdmin(c.Request.Context(), request)
			}
		}
		if err != nil {
			dto.RespondErrorFrom(c, err)
			return
		}
		dto.ResponseSuccess(c, result)
	})
}
