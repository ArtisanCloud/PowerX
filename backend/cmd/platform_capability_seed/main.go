package main

import (
	"context"
	"flag"
	"fmt"
	"gorm.io/gorm"
	"os"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	integrationgateway "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	iamrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/iam"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func main() {
	defaultConfigPath := strings.TrimSpace(os.Getenv("POWERX_CONFIG"))
	if defaultConfigPath == "" {
		defaultConfigPath = "etc/config.yaml"
	}
	configPath := flag.String("config", defaultConfigPath, "PowerX config path")
	preserve := flag.Bool("preserve-installed-state", true, "增量更新正式合同，保留现有授权开关和目录状态")
	ids := flag.String("capability-ids", "", "可选：逗号分隔的正式能力 ID；租户范围始终覆盖全部受影响租户")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatalf("load config failed: %v", err)
	}
	if _, err := cfg.Server.ParseKey(); err != nil {
		fatalf("read server.secret_key failed: %v", err)
	}
	config.GlobalConfig = cfg
	apikeypermissions.SetIntroducedVersion(cfg.EffectiveSystemVersion())

	db, err := database.Connect(cfg.Database)
	if err != nil {
		fatalf("connect database failed: %v", err)
	}

	ctx := context.Background()
	// Publish exact permission metadata alongside the capability catalog. This
	// does not attach permissions to API keys or change credential grants.
	var capabilityIDs []string
	if strings.TrimSpace(*ids) != "" {
		for _, id := range strings.Split(*ids, ",") {
			capabilityIDs = append(capabilityIDs, strings.TrimSpace(id))
		}
	}
	if len(capabilityIDs) > 0 && !*preserve {
		fatalf("scoped upgrades require preserve-installed-state")
	}
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if *preserve {
			if err := apikeypermissions.EnsureUpgradePermissionsForCapabilities(ctx, tx, capabilityIDs); err != nil {
				return err
			}
		} else if err := apikeypermissions.EnsureTemplatePermissions(ctx, iamrepo.NewPermissionRepository(tx)); err != nil {
			return err
		}
		return integrationgateway.NewBaseCapabilitySeeder(integrationgateway.BaseCapabilitySeederOptions{
			PreserveInstalledState: *preserve,
			CapabilityIDs:          capabilityIDs,
			RecordRepo:             caprepo.NewCapabilityRecordRepository(tx, nil),
			RegistryRepo:           caprepo.NewCapabilityRegistryRepository(tx),
			TenantRepo:             tenantrepo.NewTenantRepository(tx),
			Logger:                 logger.GetGlobalLogger(),
		}).Ensure(ctx)
	})
	if err != nil {
		fatalf("seed platform capabilities failed: %v", err)
	}
	logger.InfoF(ctx, "platform capability seed ok")
}

func fatalf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintln(os.Stderr, message)
	logger.Error(context.Background(), message)
	os.Exit(1)
}
