package seed

import (
	"context"
	"fmt"

	metasvc "github.com/ArtisanCloud/PowerX/internal/service/metadata"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"gorm.io/gorm"
)

var metadataSeedFiles = []struct {
	path             string
	requireCanonical bool
}{
	{path: metasvc.DefaultSeedPath, requireCanonical: true},
	{path: "config/metadata_governance/enterprise_seed.yaml"},
}

func SeedMetadataGovernance(ctx context.Context, db *gorm.DB) error {
	tenantUUIDs, err := tenantrepo.NewTenantRepository(db).ListActiveUUIDs(ctx)
	if err != nil {
		return fmt.Errorf("list active tenants for metadata seed: %w", err)
	}
	if len(tenantUUIDs) == 0 {
		return fmt.Errorf("metadata seed requires at least one active tenant")
	}

	service, err := metasvc.NewSeedService(metasvc.SeedServiceOptions{DB: db})
	if err != nil {
		return fmt.Errorf("initialize metadata seed service: %w", err)
	}
	for _, tenantUUID := range tenantUUIDs {
		for _, seedFile := range metadataSeedFiles {
			result, seed, err := service.Execute(ctx, metasvc.SeedExecutionInput{
				TenantUUID:                  tenantUUID,
				SeedPath:                    seedFile.path,
				RequireCanonicalDefinitions: seedFile.requireCanonical,
			})
			if err != nil {
				return fmt.Errorf("seed metadata tenant=%s file=%s: %w", tenantUUID, seedFile.path, err)
			}
			logger.InfoF(logger.WithLogFields(ctx, map[string]interface{}{"module": "legacy"}), "[seed] metadata ready tenant=%s module=%s dictionaries=%d items=%d taxonomies=%d nodes=%d resource_types=%d tags=%d",
				tenantUUID, seed.Module, result.DictionaryNamespaces, result.DictionaryItems, result.Taxonomies, result.TaxonomyNodes, result.ResourceTypes, result.Tags)
		}
	}
	return nil
}
