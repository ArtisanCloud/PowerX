package knowledge_space

import (
	"context"
	"encoding/json"
	"time"

	dbm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	knowledge "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/knowledge"
	"gorm.io/gorm"
)

func (s *Service) insertAuditTrail(ctx context.Context, tx *gorm.DB, space *models.KnowledgeSpace, action, actor string, payload map[string]any) error {
	auditRepo := knowledge.NewAuditTrailRepository(tx)
	entry := &models.AuditTrailEntry{
		SpaceUUID:     space.UUID,
		Action:        action,
		Actor:         actor,
		Metadata:      marshalJSON(payload),
		OccurredAt:    s.clock(),
		RollbackToken: space.AuditToken,
		PayloadHash:   computePayloadHash(payload),
	}
	if _, err := auditRepo.Create(ctx, entry); err != nil {
		return err
	}
	s.emitAuditEvent(ctx, space, action, payload)
	return nil
}

func (s *Service) emitAuditEvent(ctx context.Context, space *models.KnowledgeSpace, action string, payload map[string]any) {
	if s.inst != nil {
		raw := marshalJSON(payload)
		s.inst.Audit(ctx, &dbm.AuditEvent{
			OccurredAt:   time.Now(),
			Source:       "knowledge_space",
			Operation:    action,
			ResourceType: "knowledge_space",
			ResourceID:   space.UUID.String(),
			Outcome:      "SUCCESS",
			Severity:     "INFO",
			Meta:         raw,
		})
	}
}

func (s *Service) publishEvent(ctx context.Context, verb string, space *models.KnowledgeSpace) {
	if s.bus == nil || space == nil {
		return
	}
	payload := map[string]any{
		"space_id":      space.UUID.String(),
		"tenant_id":     space.TenantID.String(),
		"space_name":    space.SpaceName,
		"status":        space.Status,
		"policy_id":     space.PolicyTemplateVersionID,
		"event":         verb,
		"department":    space.DepartmentCode,
		"audit_token":   space.AuditToken,
		"quota_cpu":     space.QuotaCPU,
		"quota_storage": space.QuotaStorageGB,
	}
	topic := s.cfg.EventTopics.Provisioning
	if topic == "" {
		topic = "knowledge.space.provisioning"
	}
	s.bus.Publish(topic, payload, ctx)
}

func marshalJSON(payload map[string]any) []byte {
	raw, _ := json.Marshal(payload)
	return raw
}
