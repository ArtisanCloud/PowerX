package router

import (
	"context"
	"strings"

	identity "github.com/ArtisanCloud/PowerX/internal/service/plugin_identity"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
)

type RequestIdentityIssuer interface {
	Issue(context.Context, identity.IssueInput) (string, error)
}

func (r *DynamicRouter) BindRequestIdentityIssuer(issuer RequestIdentityIssuer) {
	r.identityIssuer = issuer
}
func (r *DynamicRouter) issueRequestIdentity(c *gin.Context, in identity.IssueInput) (string, bool) {
	// User identity tickets attest a live user session. A service API Key has
	// already passed the route grant gate and travels as a signed plugin token;
	// it must not be promoted to a user or require a fabricated user session.
	if reqctx.AuthenticatedAPIKeyHash(c.Request.Context()) != "" {
		for _, platform := range in.Claims.Platforms {
			if platform == "api_key" {
				return "", true
			}
		}
	}
	if r.identityIssuer == nil {
		return "", true
	} // Explicitly unbound router fixtures; production always binds.
	ticket, err := r.identityIssuer.Issue(c.Request.Context(), in)
	if err != nil {
		dto.RespondErrorFrom(c, err)
		c.Abort()
		return "", false
	}
	return ticket, true
}
func identityTransport(c *gin.Context, gatePath string) string {
	if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
		return "ws_upgrade"
	}
	if strings.HasSuffix(gatePath, "/runtime/ws-bus/grant") {
		return "ws_grant"
	}
	return "http"
}
