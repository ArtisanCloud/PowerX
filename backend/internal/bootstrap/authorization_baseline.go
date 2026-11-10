package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/authorizationbaseline"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/rootrole"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

func initializeAuthorizationBaselines(ctx context.Context, cfg *config.Config, db *gorm.DB) error {
	if cfg.Recovery.ReadOnly() || os.Getenv("POWERX_COORDINATOR_MAINTENANCE") == "1" {
		return nil
	}
	if err := authorizationbaseline.EnsureInitialized(db.WithContext(ctx)); err != nil {
		return fmt.Errorf("%w: IAM authorization baseline initialization: %v", ErrBootstrapDependencyUnavailable, err)
	}
	if err := rootrole.CheckCatalog(db.WithContext(ctx)); err != nil {
		if !errors.Is(err, rootrole.ErrCatalogInconsistent) {
			return fmt.Errorf("%w: IAM Root catalog diagnosis: %v", ErrBootstrapDependencyUnavailable, err)
		}
		logger.WarnF(ctx, "[iam] %v: Root catalog requires explicit maintenance preview/repair; ordinary member administration remains available; affected Root authorization baselines are not automatically repaired", err)
	}
	return nil
}
