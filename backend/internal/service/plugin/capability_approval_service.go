package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	capmodels "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	settingrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SetIndependentCapabilityApproval is an admin-only service operation. A plugin
// manifest and a service token can never approve an independent grant. Records
// are retained after revocation; re-approval creates a new approval UUID.
func (s *TenantPluginInstanceService) SetIndependentCapabilityApproval(ctx context.Context, tenantUUID, pluginID, capabilityID string, approved bool) error {
	claims := reqctx.GetClaims(ctx)
	if claims == nil || !claims.IsRoot || claims.PluginID != "" || claims.Issuer == "powerx-sts" {
		return errors.New("plugin.capability_approval_forbidden")
	}
	actor, err := uuid.Parse(claims.UserUUID)
	if err != nil || actor == uuid.Nil {
		return errors.New("plugin.capability_approval_forbidden")
	}
	tenantUUID, err = canonicalTenantUUID(tenantUUID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(pluginID) == "" || strings.TrimSpace(capabilityID) == "" {
		return errors.New("plugin.capability_approval_invalid")
	}
	return s.requireRepo().DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cfg setting.PluginInstanceConfig
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND plugin_id = ? AND key = ?", tenantUUID, pluginID, settingrepo.KeyClientCredentials).First(&cfg).Error; err != nil {
			return err
		}
		var active setting.PluginCapabilityApproval
		err := tx.Where("tenant_uuid = ? AND plugin_id = ? AND capability_id = ? AND status = ?", tenantUUID, pluginID, capabilityID, "active").First(&active).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if approved {
			var cap capmodels.CapabilityRecord
			if err := tx.Where("capability_id = ? AND status = ?", capabilityID, "published").First(&cap).Error; err != nil {
				return err
			}
			var registration capmodels.CapabilityRegistration
			if err := tx.Where("tenant_uuid = ? AND capability_id = ?", tenantUUID, capabilityID).Order("version DESC").First(&registration).Error; err != nil {
				return err
			}
			if registration.Status != "published" {
				return errors.New("plugin.capability_approval_registration_inactive")
			}
			if active.UUID == uuid.Nil {
				if err := tx.Create(&setting.PluginCapabilityApproval{TenantUUID: tenantUUID, PluginID: pluginID, CapabilityID: capabilityID, ApprovedByUserUUID: actor, Status: "active"}).Error; err != nil {
					return err
				}
			}
		} else if active.UUID != uuid.Nil {
			now := time.Now().UTC()
			if err := tx.Model(&setting.PluginCapabilityApproval{}).Where("uuid = ?", active.UUID).Updates(map[string]any{"status": "revoked", "revoked_at": now, "revoked_by_user_uuid": actor}).Error; err != nil {
				return err
			}
		}
		var doc struct {
			Required []string `json:"manifest_capabilities"`
		}
		if err := json.Unmarshal(cfg.ValueJSON, &doc); err != nil {
			return err
		}
		// Existing manifest grants are not new approvals. Revoking an independent
		// grant must also work when an unrelated registration was withdrawn.
		if err := NewTenantPluginInstanceService(tx).reconcileCredentialCapabilityGrants(ctx, tenantUUID, pluginID, &cfg, doc.Required); err != nil {
			return err
		}
		return tx.Model(&setting.PluginInstanceConfig{}).Where("tenant_uuid = ? AND plugin_id = ? AND key = ?", tenantUUID, pluginID, settingrepo.KeyClientCredentials).Update("value_json", cfg.ValueJSON).Error
	})
}
