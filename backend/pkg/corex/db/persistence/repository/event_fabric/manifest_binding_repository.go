package eventfabric

import (
	"context"
	"strings"
	"time"

	eventfabricmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/event_fabric"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ManifestBindingRepository struct {
	db *gorm.DB
}

func NewManifestBindingRepository(db *gorm.DB) *ManifestBindingRepository {
	return &ManifestBindingRepository{db: db}
}

func (r *ManifestBindingRepository) tenantKey(value string) string {
	return eventfabricmodel.NormalizeTenantKey(value)
}

func (r *ManifestBindingRepository) pluginID(value string) string {
	return strings.TrimSpace(value)
}

func (r *ManifestBindingRepository) topicKey(value string) string {
	if strings.TrimSpace(value) == "" {
		return "default"
	}
	return strings.TrimSpace(value)
}

func (r *ManifestBindingRepository) GetTopicBinding(ctx context.Context, tenantKey, pluginID, topicKey string) (*eventfabricmodel.TopicManifestBinding, error) {
	var record eventfabricmodel.TopicManifestBinding
	err := r.db.WithContext(ctx).
		Where("tenant_key = ? AND plugin_id = ? AND topic_key = ?", r.tenantKey(tenantKey), r.pluginID(pluginID), r.topicKey(topicKey)).
		Take(&record).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *ManifestBindingRepository) UpsertTopicBinding(ctx context.Context, record *eventfabricmodel.TopicManifestBinding) error {
	if record == nil {
		return nil
	}
	record.TenantKey = r.tenantKey(record.TenantKey)
	record.PluginID = r.pluginID(record.PluginID)
	record.TopicKey = r.topicKey(record.TopicKey)
	if record.LastAppliedAt.IsZero() {
		record.LastAppliedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_key"},
			{Name: "plugin_id"},
			{Name: "topic_key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"namespace", "name", "full_topic", "topic_uuid", "last_applied_at", "updated_at"}),
	}).Create(record).Error
}

func (r *ManifestBindingRepository) GetAclBinding(ctx context.Context, tenantKey, pluginID, topicKey, principalType, principalID string) (*eventfabricmodel.AclManifestBinding, error) {
	var record eventfabricmodel.AclManifestBinding
	err := r.db.WithContext(ctx).
		Where("tenant_key = ? AND plugin_id = ? AND topic_key = ? AND principal_type = ? AND principal_id = ?",
			r.tenantKey(tenantKey), r.pluginID(pluginID), r.topicKey(topicKey),
			strings.ToLower(strings.TrimSpace(principalType)), strings.TrimSpace(principalID)).
		Take(&record).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *ManifestBindingRepository) UpsertAclBinding(ctx context.Context, record *eventfabricmodel.AclManifestBinding) error {
	if record == nil {
		return nil
	}
	record.TenantKey = r.tenantKey(record.TenantKey)
	record.PluginID = r.pluginID(record.PluginID)
	record.TopicKey = r.topicKey(record.TopicKey)
	record.PrincipalType = strings.ToLower(strings.TrimSpace(record.PrincipalType))
	record.PrincipalID = strings.TrimSpace(record.PrincipalID)
	if record.LastAppliedAt.IsZero() {
		record.LastAppliedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_key"},
			{Name: "plugin_id"},
			{Name: "topic_key"},
			{Name: "principal_type"},
			{Name: "principal_id"},
		},
		DoUpdates: clause.Assignments(map[string]any{
			"actions":         record.Actions,
			"actions_hash":    record.ActionsHash,
			"last_applied_at": record.LastAppliedAt,
			"updated_at":      time.Now().UTC(),
		}),
	}).Create(record).Error
}
