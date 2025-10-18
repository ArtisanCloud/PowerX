package event_fabric

import (
	"fmt"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/event_fabric/acl"
	directory "github.com/ArtisanCloud/PowerX/internal/service/event_fabric/directory"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/gin-gonic/gin"
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
	TenantID      string           `json:"tenant_id"`
	TopicFullName string           `json:"topic_full_name"`
	Grants        []bindingRequest `json:"grants"`
	Revokes       []bindingRequest `json:"revokes"`
}

func (h *AdminACLHandler) UpsertBindings(c *gin.Context) {
	if h.service == nil || h.directory == nil {
		dto.RespondErrorFrom(c, dto.NewInternal("service unavailable", nil))
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

	topic, err := h.directory.FindTopicByFullName(c.Request.Context(), tenantKey, namespace, name)
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
			TenantID:      req.TenantID,
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
			TenantID:    req.TenantID,
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

	tenantID := strings.TrimSpace(c.Query("tenant_id"))
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
		topic, err := h.directory.FindTopicByFullName(c.Request.Context(), tenantKey, namespace, name)
		if err != nil {
			dto.RespondErrorFrom(c, dto.NewInternal("lookup topic failed", err))
			return
		}
		if topic == nil {
			dto.RespondErrorFrom(c, dto.NewNotFound("topic not found", nil))
			return
		}
		topicUUID = topic.UUID.String()
		if tenantID == "" {
			tenantID = topic.TenantKey
		}
	}

	bindings, err := h.service.ListBindings(c.Request.Context(), acl.ListRequest{
		TenantID:  tenantID,
		TopicUUID: topicUUID,
	})
	if err != nil {
		dto.RespondErrorFrom(c, dto.NewInternal("list acl failed", err))
		return
	}

	dto.ResponseSuccess(c, map[string]interface{}{
		"items": bindings,
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
