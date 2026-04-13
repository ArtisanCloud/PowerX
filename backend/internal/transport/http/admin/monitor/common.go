package monitor

import (
	monitorlogs "github.com/ArtisanCloud/PowerX/internal/service/monitor_logs"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/gin-gonic/gin"
)

type handler struct {
	svc *monitorlogs.Service
}

func NewHandler() *handler {
	return &handler{svc: monitorlogs.NewService()}
}

func resolveOperator(c *gin.Context) string {
	ctx := c.Request.Context()
	if reqctx.IsRoot(ctx) {
		return "root"
	}
	if reqctx.GetMemberID(ctx) > 0 {
		return "member"
	}
	return "system"
}
