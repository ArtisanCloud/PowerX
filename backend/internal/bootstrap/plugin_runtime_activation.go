package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	dbsetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tenantmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	reposetting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Only an explicit Root lifecycle action may restore the existing system runtime
// binding. Startup and credential reads must never resume drained tenant usage.
// Enable handlers must leave ready drain records intact: the lifecycle snapshots
// them here and restores that checkpoint if any subsequent startup step fails.
func preparePluginRuntimeEnable(ctx context.Context, deps *shared.Deps, pluginID string) (func(context.Context) error, error) {
	if !reqctx.IsRoot(ctx) {
		return nil, nil
	}
	var previous dbsetting.PluginInstanceConfig
	var readyJobs []dbsetting.PluginDrainJob
	now := time.Now().UTC().Truncate(time.Microsecond)
	changed := false
	err := deps.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenant tenantmodel.Tenant
		if err := tx.Where("key = ?", tenantmodel.SystemTenantKey).First(&tenant).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("tenant_uuid = ? AND plugin_id = ? AND key = ?", tenant.UUID.String(), pluginID, reposetting.KeyClientCredentials).First(&previous).Error; err != nil {
			return err
		}
		var jobs []dbsetting.PluginDrainJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("plugin_id = ? AND status IN ?", pluginID, []string{
			dbsetting.PluginDrainJobStatusRequested, dbsetting.PluginDrainJobStatusBlockingNewUsage,
			dbsetting.PluginDrainJobStatusDraining, dbsetting.PluginDrainJobStatusReadyToUninstall,
		}).Find(&jobs).Error; err != nil {
			return err
		}
		for _, job := range jobs {
			if job.Status != dbsetting.PluginDrainJobStatusReadyToUninstall {
				return fmt.Errorf("PLUGIN_DRAIN_NOT_READY")
			}
			readyJobs = append(readyJobs, job)
		}
		for _, job := range readyJobs {
			if err := tx.Model(&dbsetting.PluginDrainJob{}).Where("id = ?", job.ID).Updates(map[string]any{
				"status": dbsetting.PluginDrainJobStatusCompleted, "completed_at": now, "updated_at": now,
			}).Error; err != nil {
				return err
			}
		}
		if !previous.Enabled {
			changed = true
			return tx.Model(&dbsetting.PluginInstanceConfig{}).Where("id = ?", previous.ID).Updates(map[string]any{
				"enabled": true, "status": dbsetting.PluginInstanceStatusEnabled,
				"drain_job_id": "", "drain_requested_at": nil, "drained_at": nil, "updated_at": now,
			}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return func(rollbackCtx context.Context) error {
		return deps.DB.WithContext(rollbackCtx).Transaction(func(tx *gorm.DB) error {
			if changed {
				result := tx.Model(&dbsetting.PluginInstanceConfig{}).Where("id = ? AND updated_at = ?", previous.ID, now).Updates(map[string]any{
					"enabled": previous.Enabled, "status": previous.Status, "drain_job_id": previous.DrainJobID,
					"drain_requested_at": previous.DrainRequestedAt, "drained_at": previous.DrainedAt, "updated_at": previous.UpdatedAt,
				})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return fmt.Errorf("PLUGIN_RUNTIME_ACTIVATION_CHANGED")
				}
			}
			for _, job := range readyJobs {
				result := tx.Model(&dbsetting.PluginDrainJob{}).Where("id = ? AND status = ? AND updated_at = ?", job.ID, dbsetting.PluginDrainJobStatusCompleted, now).Updates(map[string]any{
					"status": job.Status, "completed_at": job.CompletedAt, "updated_at": job.UpdatedAt,
				})
				if result.Error != nil {
					return result.Error
				}
				if result.RowsAffected != 1 {
					return fmt.Errorf("PLUGIN_RUNTIME_ACTIVATION_CHANGED")
				}
			}
			return nil
		})
	}, nil
}
