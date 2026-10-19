package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/authorizationbaseline"
	"gorm.io/gorm"
)

func initializeAuthorizationBaselines(ctx context.Context, cfg *config.Config, db *gorm.DB) error {
	if cfg.Recovery.ReadOnly() || os.Getenv("POWERX_COORDINATOR_MAINTENANCE") == "1" {
		return nil
	}
	if err := authorizationbaseline.EnsureInitialized(db.WithContext(ctx)); err != nil {
		return fmt.Errorf("%w: IAM authorization baseline initialization: %v", ErrBootstrapDependencyUnavailable, err)
	}
	return nil
}
