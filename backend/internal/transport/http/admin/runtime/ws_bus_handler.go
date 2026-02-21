package runtime

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	aclservice "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	eventshared "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/shared"
	"github.com/ArtisanCloud/PowerX/internal/transport/websocket/bus"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	eventfabricrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type wsBusPublishRequest struct {
	Topic   string `json:"topic"`
	Payload any    `json:"payload"`
	TraceID string `json:"trace_id"`
}

type wsBusRegisterRequest struct {
	Topics  []string `json:"topics"`
	Actions []string `json:"actions"`
}

type wsBusHandler struct {
	authorizer bus.Authorizer
	topics     wsTopicLookup
	acl        wsACLGrantService
}

type wsTopicLookup interface {
	FindByComposite(ctx *gin.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error)
}

type wsACLGrantService interface {
	Grant(ctx *gin.Context, req aclservice.GrantRequest) ([]*aclservice.Binding, error)
}

func newWSBusHandler(deps *shared.Deps) *wsBusHandler {
	var authorizer bus.Authorizer
	var topics wsTopicLookup
	var aclSvc wsACLGrantService
	if deps != nil && deps.DB != nil {
		authorizer = bus.NewDefaultAuthorizer(deps.DB)
		topics = newGinTopicLookup(eventshared.NewCachedTopicLookup(eventfabricrepo.NewTopicRepository(deps.DB), eventshared.CachedTopicLookupOptions{}))
	}
	if deps != nil && deps.EventFabric != nil && deps.EventFabric.ACL != nil {
		aclSvc = newGinACLGrantService(deps.EventFabric.ACL)
	}
	return &wsBusHandler{authorizer: authorizer, topics: topics, acl: aclSvc}
}

func (h *wsBusHandler) register(c *gin.Context) {
	var req wsBusRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
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

	registered := normalizeRegisterTopics(req.Topics)
	if len(registered) == 0 {
		dto.ResponseError(c, http.StatusBadRequest, "topics required", nil)
		return
	}

	if h.topics != nil {
		actions := normalizeRegisterActions(req.Actions)
		principalID := buildWSPrincipalID(memberID, reqctx.GetUserID(c.Request.Context()))
		if !isRoot && principalID == "" {
			dto.ResponseError(c, http.StatusForbidden, "principal required", nil)
			return
		}

		bound := make([]string, 0, len(registered))
		for _, topicKey := range registered {
			topicDef, err := resolveTopicForRegister(c, h.topics, strings.TrimSpace(tenantUUID), topicKey)
			if err != nil {
				dto.ResponseError(c, http.StatusBadRequest, "invalid topic", err)
				return
			}
			if topicDef == nil {
				dto.ResponseError(c, http.StatusNotFound, "topic not found", fmt.Errorf("topic %s not found", topicKey))
				return
			}

			if h.acl != nil {
				if _, err := h.acl.Grant(c, aclservice.GrantRequest{
					TenantUUID:    strings.TrimSpace(tenantUUID),
					TopicUUID:     topicDef.UUID.String(),
					PrincipalType: "member",
					PrincipalID:   principalID,
					Actions:       actions,
					OperatorID:    principalID,
					Justification: "ws bus register",
					AuditRef:      reqctx.GetTraceID(c.Request.Context()),
					ExpiresAt:     nil,
				}); err != nil {
					dto.ResponseError(c, http.StatusForbidden, "acl grant failed", err)
					return
				}
			}
			bound = append(bound, topicKey)
		}

		logger.InfoF(c.Request.Context(), "[ws-bus] register via registry tenant=%s topics=%v actions=%v", strings.TrimSpace(tenantUUID), bound, actions)
		dto.ResponseSuccessWithStatusAndPayload(c, http.StatusOK, map[string]interface{}{
			"tenant_uuid": strings.TrimSpace(tenantUUID),
			"topics":      bound,
			"actions":     actions,
			"mode":        "registry_acl",
		})
		return
	}

	registered = bus.RegisterPublishTopics(tenantUUID, registered)

	logger.InfoF(c.Request.Context(), "[ws-bus] register topics tenant=%s topics=%v", strings.TrimSpace(tenantUUID), registered)
	dto.ResponseSuccessWithStatusAndPayload(c, http.StatusOK, map[string]interface{}{
		"tenant_uuid": strings.TrimSpace(tenantUUID),
		"topics":      registered,
		"mode":        "compat_dynamic",
	})
}

func (h *wsBusHandler) publish(c *gin.Context) {
	var req wsBusPublishRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.ResponseError(c, http.StatusBadRequest, "invalid request", err)
		return
	}
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

	reqTopic := strings.TrimSpace(req.Topic)
	if reqTopic == "" {
		dto.ResponseError(c, http.StatusBadRequest, "topic required", nil)
		return
	}
	if h.authorizer != nil {
		if err := h.authorizer.AuthorizePublish(c.Request.Context(), bus.PublishAuthorizeInput{
			TenantUUID: tenantUUID,
			MemberID:   memberID,
			UserID:     reqctx.GetUserID(c.Request.Context()),
			IsRoot:     isRoot,
			Topic:      reqTopic,
		}); err != nil {
			logger.DebugF(c.Request.Context(), "[ws-bus] publish rejected tenant=%s topic=%s err=%v", strings.TrimSpace(tenantUUID), reqTopic, err)
			dto.ResponseError(c, http.StatusForbidden, "topic not allowed", err)
			return
		}
	} else {
		allowed, whitelistHit, dynamicHit := bus.PublishTopicCheck(tenantUUID, reqTopic)
		if !allowed {
			logger.DebugF(
				c.Request.Context(),
				"[ws-bus] publish rejected tenant=%s topic=%s whitelist=%t dynamic=%t",
				strings.TrimSpace(tenantUUID),
				reqTopic,
				whitelistHit,
				dynamicHit,
			)
			dto.ResponseError(c, http.StatusForbidden, "topic not allowed", nil)
			return
		}
	}

	traceID := strings.TrimSpace(req.TraceID)
	if traceID == "" {
		traceID = reqctx.GetTraceID(c.Request.Context())
	}

	bus.DefaultHub.Publish(strings.TrimSpace(tenantUUID), reqTopic, req.Payload, traceID)
	logger.InfoF(c.Request.Context(), "[ws-bus] publish topic=%s tenant=%s trace_id=%s", reqTopic, strings.TrimSpace(tenantUUID), traceID)

	dto.ResponseSuccessWithStatusAndPayload(c, http.StatusOK, map[string]interface{}{
		"topic":       reqTopic,
		"tenant_uuid": strings.TrimSpace(tenantUUID),
	})
}

type ginTopicLookup struct {
	base eventshared.TopicLookup
}

func newGinTopicLookup(base eventshared.TopicLookup) *ginTopicLookup {
	if base == nil {
		return nil
	}
	return &ginTopicLookup{base: base}
}

func (l *ginTopicLookup) FindByComposite(ctx *gin.Context, tenantKey, namespace, name string) (*eventfabricmodel.TopicDefinition, error) {
	if l == nil || l.base == nil {
		return nil, nil
	}
	return l.base.FindByComposite(ctx.Request.Context(), tenantKey, namespace, name)
}

type ginACLGrantService struct {
	base *aclservice.ACLService
}

func newGinACLGrantService(base *aclservice.ACLService) *ginACLGrantService {
	if base == nil {
		return nil
	}
	return &ginACLGrantService{base: base}
}

func (s *ginACLGrantService) Grant(ctx *gin.Context, req aclservice.GrantRequest) ([]*aclservice.Binding, error) {
	if s == nil || s.base == nil {
		return nil, nil
	}
	return s.base.Grant(ctx.Request.Context(), req)
}

func normalizeRegisterActions(actions []string) []aclservice.PrincipalAction {
	if len(actions) == 0 {
		return []aclservice.PrincipalAction{aclservice.PrincipalActionPublish, aclservice.PrincipalActionSubscribe}
	}
	unique := make(map[aclservice.PrincipalAction]struct{})
	for _, action := range actions {
		token := strings.ToLower(strings.TrimSpace(action))
		switch token {
		case string(aclservice.PrincipalActionPublish):
			unique[aclservice.PrincipalActionPublish] = struct{}{}
		case string(aclservice.PrincipalActionSubscribe):
			unique[aclservice.PrincipalActionSubscribe] = struct{}{}
		case string(aclservice.PrincipalActionReplay):
			unique[aclservice.PrincipalActionReplay] = struct{}{}
		}
	}
	if len(unique) == 0 {
		return []aclservice.PrincipalAction{aclservice.PrincipalActionPublish, aclservice.PrincipalActionSubscribe}
	}
	out := make([]aclservice.PrincipalAction, 0, len(unique))
	for action := range unique {
		out = append(out, action)
	}
	return out
}

func normalizeRegisterTopics(topics []string) []string {
	unique := make(map[string]struct{})
	for _, topic := range topics {
		trimmed := strings.TrimSpace(topic)
		if trimmed == "" {
			continue
		}
		unique[trimmed] = struct{}{}
	}
	if len(unique) == 0 {
		return nil
	}
	out := make([]string, 0, len(unique))
	for topic := range unique {
		out = append(out, topic)
	}
	return out
}

func buildWSPrincipalID(memberID, userID uint64) string {
	if memberID > 0 {
		return fmt.Sprintf("member:%d", memberID)
	}
	if userID > 0 {
		return fmt.Sprintf("user:%d", userID)
	}
	return ""
}

func resolveTopicForRegister(c *gin.Context, store wsTopicLookup, tenantKey string, topic string) (*eventfabricmodel.TopicDefinition, error) {
	if store == nil {
		return nil, nil
	}
	topicTenant, namespace, name, err := parseRegisterTopic(topic)
	if err != nil {
		return nil, err
	}
	if topicTenant != "" && !strings.EqualFold(topicTenant, tenantKey) && !isSharedWSRegisterTopicTenant(topicTenant) {
		return nil, fmt.Errorf("topic tenant mismatch: %s", topic)
	}
	return store.FindByComposite(c, tenantKey, namespace, name)
}

func parseRegisterTopic(topic string) (tenant string, namespace string, name string, err error) {
	trimmed := strings.TrimSpace(topic)
	if trimmed == "" {
		return "", "", "", fmt.Errorf("topic required")
	}
	parts := strings.Split(trimmed, ".")
	if len(parts) < 2 {
		return "", "", "", fmt.Errorf("invalid topic format")
	}
	first := strings.TrimSpace(parts[0])
	if parsed, parseErr := uuid.Parse(first); parseErr == nil && parsed != uuid.Nil && len(parts) >= 3 {
		tenant = first
		namespace = strings.Join(parts[1:len(parts)-1], ".")
		name = strings.TrimSpace(parts[len(parts)-1])
		return tenant, namespace, name, nil
	}
	namespace = strings.Join(parts[:len(parts)-1], ".")
	name = strings.TrimSpace(parts[len(parts)-1])
	return "", namespace, name, nil
}

func isSharedWSRegisterTopicTenant(tenantKey string) bool {
	key := strings.ToLower(strings.TrimSpace(tenantKey))
	return key == "global" || key == "system"
}
