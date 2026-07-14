package seed

import (
	"context"
	"os"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	integrationgateway "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway"
	apikeypermissions "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"

	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func seedCtx() context.Context { return context.Background() }

func envOrDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func SeedCoreX(ctx context.Context, db *gorm.DB, cfg *config.Config) error {
	if cfg != nil {
		config.GlobalConfig = cfg
		apikeypermissions.SetIntroducedVersion(cfg.EffectiveSystemVersion())
	}
	db, err := database.Connect(cfg.Database)
	if err != nil {
		return err
	}

	if err = SeedRoot(db); err != nil {
		return err
	}
	if err = SeedDefaultDevAPIKeys(db); err != nil {
		return err
	}
	// Keep capability catalog aligned with the generated platform capabilities file.
	// Without this, newly generated capability IDs can exist in permission catalog
	// but still be missing from capability registry records.
	baseCapabilitySeeder := integrationgateway.NewBaseCapabilitySeeder(integrationgateway.BaseCapabilitySeederOptions{
		RecordRepo:   caprepo.NewCapabilityRecordRepository(db, nil),
		RegistryRepo: caprepo.NewCapabilityRegistryRepository(db),
		TenantRepo:   tenantrepo.NewTenantRepository(db),
		Logger:       logger.GetGlobalLogger(),
	})
	if err = baseCapabilitySeeder.Ensure(ctx); err != nil {
		return err
	}

	if err = SeedEventFabricTopics(db); err != nil {
		return err
	}
	if err = SeedEventFabricDefaultACL(db); err != nil {
		return err
	}

	if err = SeedSMEDepartments(db, "system"); err != nil {
		return err
	}

	if err = SeedSystemDefaultAgent(db); err != nil {
		return err
	}

	if err = SeedOfficialBuiltinSkills(db); err != nil {
		return err
	}
	if err = SeedDemoThirdPartySkills(db); err != nil {
		return err
	}
	if err = SeedDemoSkillInstallTasks(db); err != nil {
		return err
	}
	if err = SeedA2AReleaseReadinessDemo(db); err != nil {
		return err
	}

	if err = SeedCapabilityErrorTaxonomies(db); err != nil {
		return err
	}
	if err = SeedMetadataGovernance(ctx, db); err != nil {
		return err
	}

	if err = SeedKnowledgePolicyTemplates(db); err != nil {
		return err
	}

	if err = SeedKnowledgeProfiles(db, "system"); err != nil {
		return err
	}
	if err = SeedDefaultAIConfig(db, cfg); err != nil {
		return err
	}

	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "seed ok")

	return nil
}
