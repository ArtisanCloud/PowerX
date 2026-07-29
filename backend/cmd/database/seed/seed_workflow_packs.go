package seed

import (
	"context"
	"fmt"

	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

const workflowPackSeedConfigDir = "config/workflow_packs"

func SeedWorkflowPacks(ctx context.Context, db *gorm.DB) error {
	tenantUUIDs, err := tenantrepo.NewTenantRepository(db).ListActiveUUIDs(ctx)
	if err != nil {
		return fmt.Errorf("list active tenants for workflow pack seed: %w", err)
	}
	if len(tenantUUIDs) == 0 {
		return fmt.Errorf("workflow pack seed requires at least one active tenant")
	}

	service := workflowsvc.NewService(db, workflowsvc.ServiceOptions{})
	for _, tenantUUID := range tenantUUIDs {
		result, err := service.SeedWorkflowPacks(ctx, workflowsvc.WorkflowPackSeedInput{
			TenantUUID: tenantUUID,
			ConfigDir:  workflowPackSeedConfigDir,
		})
		if err != nil {
			return fmt.Errorf("seed workflow packs tenant=%s config_dir=%s: %w", tenantUUID, workflowPackSeedConfigDir, err)
		}
		logger.InfoF(logger.WithLogFields(ctx, map[string]interface{}{"module": "legacy"}), "[seed] workflow packs ready tenant=%s seeded=%d skipped=%d",
			tenantUUID, len(result.Seeded), len(result.Skipped))
	}
	return nil
}
