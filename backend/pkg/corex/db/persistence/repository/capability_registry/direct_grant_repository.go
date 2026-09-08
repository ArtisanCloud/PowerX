package capability_registry

import (
	"context"

	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"gorm.io/gorm"
)

// DirectGrantRepository deliberately reads current rows without the catalog
// cache. Revocation must affect an already-issued token on its next request.
type DirectGrantRepository struct{ db *gorm.DB }

func NewDirectGrantRepository(db *gorm.DB) *DirectGrantRepository {
	return &DirectGrantRepository{db: db}
}
func (r *DirectGrantRepository) Facts(ctx context.Context, tenantUUID, pluginID, capabilityID string) (bool, []byte, error) {
	var n int64
	for _, query := range []*gorm.DB{
		r.db.WithContext(ctx).Model(&tenant.Tenant{}).Where("uuid = ? AND status = ?", tenantUUID, tenant.TenantStatusActive),
		r.db.WithContext(ctx).Model(&m.CapabilityRecord{}).Where("capability_id = ? AND status = ?", capabilityID, "published"),
	} {
		if err := query.Count(&n).Error; err != nil {
			return false, nil, err
		}
		if n == 0 {
			return false, nil, nil
		}
	}
	var registration m.CapabilityRegistration
	if err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND capability_id = ?", tenantUUID, capabilityID).Order("version DESC").First(&registration).Error; err != nil {
		return false, nil, err
	}
	if registration.Status != "published" {
		return false, nil, nil
	}
	var credential setting.PluginInstanceConfig
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", tenantUUID, pluginID, "auth.credentials", true).First(&credential).Error
	return true, credential.ValueJSON, err
}
