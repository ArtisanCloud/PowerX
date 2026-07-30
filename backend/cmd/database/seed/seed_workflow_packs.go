package seed

import (
	"context"
	"fmt"

	workflowsvc "github.com/ArtisanCloud/PowerX/internal/service/workflow"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

const workflowPackSeedConfigDir = "config/workflow_packs"

func SeedWorkflowPacks(ctx context.Context, db *gorm.DB) error {
	service := workflowsvc.NewService(db, workflowsvc.ServiceOptions{})
	packs, err := service.ValidateWorkflowPackCatalog(workflowPackSeedConfigDir, nil)
	if err != nil {
		return fmt.Errorf("validate workflow pack catalog config_dir=%s: %w", workflowPackSeedConfigDir, err)
	}
	logger.InfoF(logger.WithLogFields(ctx, map[string]interface{}{"module": "legacy"}), "[seed] workflow pack catalog validated packs=%d config_dir=%s",
		len(packs), workflowPackSeedConfigDir)
	return nil
}
