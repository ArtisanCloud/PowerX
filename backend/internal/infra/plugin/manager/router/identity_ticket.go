package router

import (
	"context"
	"strings"

	identity "github.com/ArtisanCloud/PowerX/internal/service/plugin_identity"
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
