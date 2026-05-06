package bus

import (
	"net/http"
	"strings"
	"time"

	eventshared "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Handler struct {
	hub        *Hub
	authorizer Authorizer
}

type HandlerOptions struct {
	DB          *gorm.DB
	TopicLookup topicLookup
	ACLStore    aclChecker
}

func NewHandler(db *gorm.DB) *Handler {
	return NewHandlerWithOptions(HandlerOptions{DB: db})
}

func NewHandlerWithOptions(opts HandlerOptions) *Handler {
	topicLookup := opts.TopicLookup
	aclStore := opts.ACLStore
	if topicLookup == nil && opts.DB != nil {
		topicLookup = eventshared.NewCachedTopicLookup(eventfabricrepo.NewTopicRepository(opts.DB), eventshared.CachedTopicLookupOptions{Cache: nil})
	}
	if aclStore == nil && opts.DB != nil {
		aclStore = eventfabricrepo.NewAclRepository(opts.DB)
	}
	if opts.DB != nil && topicLookup != nil && aclStore != nil {
		return &Handler{
			hub: DefaultHub,
			authorizer: NewDefaultAuthorizerWithOptions(DefaultAuthorizerOptions{
				DB:         opts.DB,
				TopicStore: topicLookup,
				ACLStore:   aclStore,
				Clock:      time.Now,
			}),
		}
	}
	var authorizer Authorizer
	if opts.DB != nil {
		authorizer = NewDefaultAuthorizer(opts.DB)
	}
	return &Handler{hub: DefaultHub, authorizer: authorizer}
}

var busUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *Handler) ServeWS(c *gin.Context) {
	logger.Info(logger.WithLogFields(c.Request.Context(), map[string]interface{}{"module": "transport.wsbus"}), "wsbus_session",
		zap.String("stage", "serve_ws_enter"),
		zap.String("path", strings.TrimSpace(c.Request.URL.Path)),
		zap.String("query", strings.TrimSpace(c.Request.URL.RawQuery)),
	)
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
		logger.Info(logger.WithLogFields(c.Request.Context(), map[string]interface{}{"module": "transport.wsbus"}), "wsbus_session",
			zap.String("stage", "upgrade_failed"),
			zap.Error(err),
		)
		return
	}
	logger.Info(logger.WithLogFields(c.Request.Context(), map[string]interface{}{"module": "transport.wsbus"}), "wsbus_session",
		zap.String("stage", "upgraded"),
		zap.String("tenant_uuid", strings.TrimSpace(tenantUUID)),
		zap.Uint64("member_id", memberID),
	)

	client := NewClient(c.Request.Context(), conn, h.hub, h.authorizer)
	client.TenantUUID = strings.TrimSpace(tenantUUID)
	client.MemberID = memberID
	client.UserID = reqctx.GetUserID(c.Request.Context())
	client.IsRoot = isRoot

	h.hub.Register(client)
	logger.Info(logger.WithLogFields(c.Request.Context(), map[string]interface{}{"module": "transport.wsbus"}), "wsbus_session",
		zap.String("stage", "connected"),
		zap.String("connection_id", client.ID),
		zap.String("tenant_uuid", client.TenantUUID),
		zap.Uint64("member_id", client.MemberID),
		zap.Uint64("user_id", client.UserID),
		zap.Bool("is_root", client.IsRoot),
	)
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
