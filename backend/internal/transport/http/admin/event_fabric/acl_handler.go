package eventfabric

import (
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	directory "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AdminACLHandlerOptions struct {
	Service   *acl.ACLService
	Directory *directory.DirectoryService
}

type AdminACLHandler struct {
	service   *acl.ACLService
	directory *directory.DirectoryService
}

func NewAdminACLHandler(opts AdminACLHandlerOptions) *AdminACLHandler {
	return &AdminACLHandler{service: opts.Service, directory: opts.Directory}
}

type bindingRequest struct {
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	Action        string `json:"action"`
	ExpiresAt     string `json:"expires_at"`
	Justification string `json:"justification"`
	AuditRef      string `json:"audit_ref"`
	OperatorID    string `json:"operator_id"`
}

type aclBatchRequest struct {
	TopicFullName string           `json:"topic_full_name"`
	Grants        []bindingRequest `json:"grants"`
	Revokes       []bindingRequest `json:"revokes"`
}

func (h *AdminACLHandler) UpsertBindings(c *gin.Context) {
	if h.service == nil || h.directory == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("service unavailable", nil))
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	var req aclBatchRequest
	if err := dto.ValidateRequestWithContext(c, &req); err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid request", err))
		return
	}

	tenantKey, namespace, name, err := splitFullTopic(req.TopicFullName)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewBadRequest("invalid topic", err))
		return
	}
	if tenantKey != "" && !strings.EqualFold(tenantKey, tenantUUID) && !isSharedTopicTenant(tenantKey) {
		dto.RespondErrorFrom(c, dto.NewForbidden("tenant scope mismatch", nil))
		return
	}
	lookupTenantKey := strings.TrimSpace(tenantUUID)
	if strings.TrimSpace(tenantKey) != "" {
		lookupTenantKey = strings.TrimSpace(tenantKey)
	}

	topic, err := h.directory.FindTopicByFullName(c.Request.Context(), lookupTenantKey, namespace, name)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("lookup topic failed", err))
		return
	}
	if topic == nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("topic not found", nil))
		return
	}

	if len(req.Grants) == 0 && len(req.Revokes) == 0 {
		dto.RespondErrorFrom(c, dto.NewBadRequest("no operations specified", nil))
		return
	}

	granted := make([]*acl.Binding, 0)
	for _, grant := range req.Grants {
		actions := []acl.PrincipalAction{acl.PrincipalAction(strings.ToLower(strings.TrimSpace(grant.Action)))}
		expiresAt, err := parseTimeRef(grant.ExpiresAt)
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("invalid expires_at", err))
			return
		}
		bindings, err := h.service.Grant(c.Request.Context(), acl.GrantRequest{
			TenantUUID:    tenantUUID,
			TopicUUID:     topic.UUID.String(),
			PrincipalType: grant.PrincipalType,
			PrincipalID:   grant.PrincipalID,
			Actions:       actions,
			ExpiresAt:     expiresAt,
			Justification: grant.Justification,
			AuditRef:      grant.AuditRef,
			OperatorID:    grant.OperatorID,
		})
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("grant acl failed", err))
			return
		}
		granted = append(granted, bindings...)
	}

	revoked := make([]bindingRequest, 0, len(req.Revokes))
	for _, revoke := range req.Revokes {
		actions := []acl.PrincipalAction{acl.PrincipalAction(strings.ToLower(strings.TrimSpace(revoke.Action)))}
		if err := h.service.Revoke(c.Request.Context(), acl.RevokeRequest{
			TenantUUID:  tenantUUID,
			TopicUUID:   topic.UUID.String(),
			PrincipalID: revoke.PrincipalID,
			Actions:     actions,
			OperatorID:  revoke.OperatorID,
		}); err != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("revoke acl failed", err))
			return
		}
		revoked = append(revoked, revoke)
	}

	dto.ResponseSuccess(c, map[string]interface{}{
		"granted": granted,
		"revoked": revoked,
	})
}

func (h *AdminACLHandler) ListBindings(c *gin.Context) {
	if h.service == nil || h.directory == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("service unavailable", nil))
		return
	}

	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}
	topicFull := strings.TrimSpace(c.Query("topic_full_name"))
	topicID := strings.TrimSpace(c.Query("topic_uuid"))

	var topicUUID string
	if topicID != "" {
		topicUUID = topicID
	} else {
		tenantKey, namespace, name, err := splitFullTopic(topicFull)
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewBadRequest("invalid topic", err))
			return
		}
		if tenantKey != "" && !strings.EqualFold(tenantKey, tenantUUID) && !isSharedTopicTenant(tenantKey) {
			dto.RespondErrorFrom(c, dto.NewForbidden("tenant scope mismatch", nil))
			return
		}
		lookupTenantKey := strings.TrimSpace(tenantUUID)
		if strings.TrimSpace(tenantKey) != "" {
			lookupTenantKey = strings.TrimSpace(tenantKey)
		}
		topic, err := h.directory.FindTopicByFullName(c.Request.Context(), lookupTenantKey, namespace, name)
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewInternal("lookup topic failed", err))
			return
		}
		if topic == nil {
			dto.RespondErrorFrom(c, dto.NewNotFound("topic not found", nil))
			return
		}
		topicUUID = topic.UUID.String()
	}

	bindings, err := h.service.ListBindings(c.Request.Context(), acl.ListRequest{
		TenantUUID: tenantUUID,
		TopicUUID:  topicUUID,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list acl failed", err))
		return
	}

	dto.ResponseSuccess(c, map[string]interface{}{
		"items": bindings,
	})
}

func (h *AdminACLHandler) ListTopicRoleMatrix(c *gin.Context) {
	if h.service == nil || h.directory == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("service unavailable", nil))
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	namespace := strings.TrimSpace(c.Query("namespace"))
	if namespace == "" {
		namespace = "knowledge.space.feedback"
	}
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		name = "reprocess"
	}

	topic, err := h.findTopicWithFallback(c, tenantUUID, namespace, name)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("lookup topic failed", err))
		return
	}
	if topic == nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("topic not found", nil))
		return
	}

	bindings, err := h.service.ListBindings(c.Request.Context(), acl.ListRequest{
		TenantUUID: tenantUUID,
		TopicUUID:  topic.UUID.String(),
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list acl failed", err))
		return
	}

	actionSet := map[string]struct{}{}
	principalMap := map[string]map[string]bool{}
	for _, b := range bindings {
		if b == nil {
			continue
		}
		action := strings.TrimSpace(string(b.Action))
		if action == "" {
			continue
		}
		actionSet[action] = struct{}{}
		pid := strings.TrimSpace(b.PrincipalID)
		if pid == "" {
			continue
		}
		if _, ok := principalMap[pid]; !ok {
			principalMap[pid] = map[string]bool{}
		}
		principalMap[pid][action] = true
	}

	actions := make([]string, 0, len(actionSet))
	for action := range actionSet {
		actions = append(actions, action)
	}
	if len(actions) == 0 {
		actions = []string{"publish", "subscribe", "replay"}
	}

	principals := make([]map[string]interface{}, 0, len(principalMap))
	for principalID, permits := range principalMap {
		principals = append(principals, map[string]interface{}{
			"principal_id": principalID,
			"actions":      permits,
		})
	}

	dto.ResponseSuccess(c, map[string]interface{}{
		"topic": map[string]interface{}{
			"topic_uuid": topic.UUID.String(),
			"tenant_key": topic.TenantKey,
			"full_topic": topic.FullTopic,
			"namespace":  topic.Namespace,
			"name":       topic.Name,
		},
		"actions":    actions,
		"principals": principals,
	})
}

func (h *AdminACLHandler) ListPrincipalTopicMatrix(c *gin.Context) {
	if h.service == nil || h.directory == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("service unavailable", nil))
		return
	}
	tenantUUID, err := reqctx.RequireTenantUUIDFromGin(c)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewUnauthorized("tenant context missing", err))
		return
	}

	principalID := strings.TrimSpace(c.Query("principal_id"))
	if principalID == "" {
		dto.RespondErrorFrom(c, dto.NewBadRequest("principal_id is required", nil))
		return
	}

	namespace := strings.TrimSpace(c.Query("namespace"))
	if namespace == "" {
		namespace = "knowledge.space.feedback"
	}
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		name = "reprocess"
	}

	topic, err := h.findTopicWithFallback(c, tenantUUID, namespace, name)
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("lookup topic failed", err))
		return
	}
	if topic == nil {
		dto.RespondErrorFrom(c, dto.NewNotFound("topic not found", nil))
		return
	}

	bindings, err := h.service.ListBindings(c.Request.Context(), acl.ListRequest{
		TenantUUID: tenantUUID,
		TopicUUID:  topic.UUID.String(),
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list acl failed", err))
		return
	}

	actions := make([]string, 0, 3)
	for _, b := range bindings {
		if b == nil || strings.TrimSpace(b.PrincipalID) != principalID {
			continue
		}
		actions = append(actions, strings.TrimSpace(string(b.Action)))
	}

	dto.ResponseSuccess(c, map[string]interface{}{
		"principal_id": principalID,
		"topics": []map[string]interface{}{
			{
				"topic_uuid": topic.UUID.String(),
				"topic":      topic.FullTopic,
				"actions":    actions,
			},
		},
	})
}

func splitFullTopic(full string) (tenant, namespace, name string, err error) {
	parts := strings.Split(strings.TrimSpace(full), ".")
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("topic full name must be <tenant>.<namespace>.<name>")
	}
	tenant = parts[0]
	namespace = strings.Join(parts[1:len(parts)-1], ".")
	name = parts[len(parts)-1]
	if strings.TrimSpace(tenant) == "" || strings.TrimSpace(namespace) == "" || strings.TrimSpace(name) == "" {
		return "", "", "", fmt.Errorf("topic full name contains empty segment")
	}
	return tenant, namespace, name, nil
}

func parseTimeRef(value string) (*time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, err
	}
	return &ts, nil
}

func isSharedTopicTenant(tenantKey string) bool {
	key := strings.ToLower(strings.TrimSpace(tenantKey))
	return key == "global" || key == "system"
}

func normalizeTopicFullName(input string, fallbackTenant string) (string, error) {
	if strings.TrimSpace(input) == "" {
		return "", fmt.Errorf("topic required")
	}
	tenant, namespace, name, err := splitFullTopic(input)
	if err == nil {
		if tenant == "" {
			tenant = strings.TrimSpace(fallbackTenant)
		}
		if tenant == "" {
			tenant = "global"
		}
		return fmt.Sprintf("%s.%s.%s", tenant, namespace, name), nil
	}
	parts := strings.Split(strings.TrimSpace(input), ".")
	if len(parts) < 2 {
		return "", err
	}
	namespace = strings.Join(parts[:len(parts)-1], ".")
	name = parts[len(parts)-1]
	tenant = strings.TrimSpace(fallbackTenant)
	if tenant == "" {
		tenant = "global"
	}
	return fmt.Sprintf("%s.%s.%s", tenant, strings.TrimSpace(namespace), strings.TrimSpace(name)), nil
}

func parseTopicUUID(value string) (uuid.UUID, error) {
	id := strings.TrimSpace(value)
	if id == "" {
		return uuid.Nil, fmt.Errorf("topic_uuid is required")
	}
	return uuid.Parse(id)
}

func (h *AdminACLHandler) findTopicWithFallback(c *gin.Context, tenantUUID, namespace, name string) (*eventfabricmodel.TopicDefinition, error) {
	lookupOrder := []string{strings.TrimSpace(tenantUUID), "global", "system"}
	seen := map[string]struct{}{}
	for _, tenantKey := range lookupOrder {
		key := strings.TrimSpace(tenantKey)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		topic, err := h.directory.FindTopicByFullName(c.Request.Context(), key, namespace, name)
		if err != nil {
			return nil, err
		}
		if topic != nil {
			return topic, nil
		}
	}
	return nil, nil
}
