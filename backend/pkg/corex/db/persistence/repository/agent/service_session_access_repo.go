package agent

import (
	"context"

	legacyagent "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/google/uuid"
)

// GrantFacts loads current storage facts. The service decides authorization;
// no claims allow-list or cached grant can replace these reads.
func (r *ServiceSessionRepository) GrantFacts(ctx context.Context, tenant uuid.UUID, pluginID, capability string) (bool, []byte, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&capmodel.CapabilityRecord{}).Where("capability_id = ? AND status = ?", capability, "published").Count(&n).Error; err != nil {
		return false, nil, err
	}
	if n == 0 {
		return false, nil, nil
	}
	var registration capmodel.CapabilityRegistration
	if err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND capability_id = ?", tenant, capability).Order("version DESC").First(&registration).Error; err != nil {
		return false, nil, err
	}
	if registration.Status != "published" {
		return false, nil, nil
	}
	var cfg setting.PluginInstanceConfig
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND plugin_id = ? AND key = ? AND enabled = ?", tenant, pluginID, "auth.credentials", true).First(&cfg).Error
	return true, cfg.ValueJSON, err
}

func (r *ServiceSessionRepository) Agent(ctx context.Context, tenant, agentUUID uuid.UUID) (*legacyagent.Agent, error) {
	var agent legacyagent.Agent
	err := r.db.WithContext(ctx).Where("tenant_uuid = ? AND uuid = ?", tenant, agentUUID).First(&agent).Error
	return &agent, err
}
