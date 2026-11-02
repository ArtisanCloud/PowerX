package capability_registry

import (
	"context"
	"errors"
	"time"

	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	"github.com/google/uuid"
)

// MarkPublicationResult 固定更新发布结果，保留租户边界且不声称消费业务已完成。
func (r *CapabilityEventPublicationRepository) MarkPublicationResult(ctx context.Context, tenant string, id uuid.UUID, status, receipt string, at time.Time) error {
	if tenant == "" || id == uuid.Nil || len(receipt) > 512 {
		return errors.New("invalid publication identity")
	}
	if status != "published" && status != "dispatched" && status != "unrouted" && status != "failed" && status != "unconfirmed" {
		return errors.New("invalid publication result")
	}
	r.retentionOnce.Do(func() {
		r.retentionChecked = r.db.Migrator().HasColumn(&models.CapabilityEventPublication{}, "record_class") && r.db.Migrator().HasColumn(&models.CapabilityEventPublication{}, "terminal_at") && r.db.Migrator().HasColumn(&models.CapabilityEventPublication{}, "delivery_receipt")
	})
	fields := map[string]any{"status": status, "attempts": 1, "last_error": ""}
	if status == "failed" {
		fields["last_error"] = "publication_transport_failed"
	}
	if status == "unconfirmed" {
		fields["last_error"] = "publication_acknowledgement_unavailable"
	}
	if status == "published" || status == "dispatched" || status == "unrouted" {
		fields["publish_time"] = at
	}
	if r.retentionChecked {
		fields["delivery_receipt"] = receipt
		if status != "unconfirmed" {
			fields["terminal_at"] = at
		}
	}
	return r.db.WithContext(ctx).Model(&models.CapabilityEventPublication{}).Where("tenant_uuid = ? AND uuid = ? AND status = ?", tenant, id, "pending").Updates(fields).Error
}
func (r *InvocationTraceRepository) LinkPublication(ctx context.Context, tenant string, id, publication uuid.UUID, confirmed bool) error {
	if tenant == "" || id == uuid.Nil || publication == uuid.Nil {
		return errors.New("invalid publication link")
	}
	return r.db.WithContext(ctx).Model(&models.InvocationTrace{}).Where("tenant_uuid = ? AND uuid = ?", tenant, id).Updates(map[string]any{"event_publication_id": publication, "event_published": confirmed}).Error
}
