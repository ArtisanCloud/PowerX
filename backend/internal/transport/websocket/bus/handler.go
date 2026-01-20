package bus

import (
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

type Handler struct {
	hub        *Hub
	authorizer Authorizer
}

func NewHandler(db *gorm.DB) *Handler {
	var authorizer Authorizer
	if db != nil {
		authorizer = NewDefaultAuthorizer(db)
	}
	return &Handler{
		hub:        DefaultHub,
		authorizer: authorizer,
	}
}

var busUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *Handler) ServeWS(c *gin.Context) {
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "tenant_uuid required", err)
		return
	}
	memberID := reqctx.GetMemberID(c.Request.Context())
	isRoot := reqctx.IsRoot(c.Request.Context())
	if !isRoot && memberID == 0 {
		dto.ResponseError(c, http.StatusForbidden, "member required", nil)
		return
	}

	conn, err := busUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := NewClient(c.Request.Context(), conn, h.hub, h.authorizer)
	client.TenantUUID = strings.TrimSpace(tenantUUID)
	client.MemberID = memberID
	client.UserID = reqctx.GetUserID(c.Request.Context())
	client.IsRoot = isRoot

	h.hub.Register(client)
	_ = sendWelcome(client)
	client.Run()
}

func sendWelcome(client *Client) error {
	if client == nil {
		return nil
	}
	env, err := dto.NewWSBusEnvelope(dto.WSBusTypeWelcome, "", dto.WSBusWelcomePayload{
		Protocol:     dto.ProtocolVersion,
		Server:       "powerx-ws-bus",
		HeartbeatSec: 25,
	}, "")
	if err != nil {
		return err
	}
	client.sendEnvelope(env)
	return nil
}
