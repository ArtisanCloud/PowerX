package main

import (
	"context"
	"flag"
	"os"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	integrationgateway "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

func main() {
	defaultConfigPath := strings.TrimSpace(os.Getenv("POWERX_CONFIG"))
	if defaultConfigPath == "" {
		defaultConfigPath = "etc/config.yaml"
	}
	configPath := flag.String("config", defaultConfigPath, "PowerX config path")
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
	seeder := integrationgateway.NewBaseCapabilitySeeder(integrationgateway.BaseCapabilitySeederOptions{
		RecordRepo:   caprepo.NewCapabilityRecordRepository(db, nil),
		RegistryRepo: caprepo.NewCapabilityRegistryRepository(db),
		TenantRepo:   tenantrepo.NewTenantRepository(db),
		Logger:       logger.GetGlobalLogger(),
	})
	if err := seeder.Ensure(ctx); err != nil {
		fatalf("seed platform capabilities failed: %v", err)
	}
	logger.InfoF(ctx, "platform capability seed ok")
}

func fatalf(format string, args ...any) {
	logger.ErrorF(context.Background(), format, args...)
	os.Exit(1)
}
