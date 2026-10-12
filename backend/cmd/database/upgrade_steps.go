package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/persistence"
	gateway "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway"
	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/apikeypermissions"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	caprepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	tenantrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/tenant"
	"gorm.io/gorm"
)

const supportedUpgradeSource = "3d792fd7ea2426825970bceceb0d6e9db46211ef"

//go:embed upgrade_payloads.json
var upgradePayloads []byte

type upgradePayload struct {
	Name     string `json:"name"`
	Checksum string `json:"checksum"`
}

// 内容寻址的迁移 ID：发布后载荷改变必然产生新 ID，不能覆盖旧履历。
// 所有载荷由源码清单生成并由构建门禁 --check 校验，不接受调用方脚本。
func installedUpgradeSteps(cfg *config.Config) ([]database.UpgradeStep, string, error) {
	var payloads []upgradePayload
	if err := json.Unmarshal(upgradePayloads, &payloads); err != nil {
		return nil, "", err
	}
	callbacks := map[string]func(context.Context, *gorm.DB) error{
		"core": func(ctx context.Context, db *gorm.DB) error { return database.MigrateCoreModels(db.WithContext(ctx)) },
		"agent": func(ctx context.Context, db *gorm.DB) error {
			return persistence.MigrateAgentModels(db.WithContext(ctx))
		},
		"knowledge": func(ctx context.Context, db *gorm.DB) error { return MigrateKnowledgeDatabase(ctx, db, cfg) },
		"catalog": func(ctx context.Context, db *gorm.DB) error {
			// 仅同步平台目录；不调用 SeedRoot/SeedCoreX、默认 Profile 或授权种子。
			if err := apikeypermissions.EnsureUpgradePermissions(ctx, db); err != nil {
				return err
			}
			return gateway.NewBaseCapabilitySeeder(gateway.BaseCapabilitySeederOptions{
				PreserveInstalledState: true,
				RecordRepo:             caprepo.NewCapabilityRecordRepository(db, nil), RegistryRepo: caprepo.NewCapabilityRegistryRepository(db), TenantRepo: tenantrepo.NewTenantRepository(db),
			}).Ensure(ctx)
		},
	}
	steps := []database.UpgradeStep{}
	for _, p := range payloads {
		if len(p.Checksum) != 64 || callbacks[p.Name] == nil {
			return nil, "", fmt.Errorf("invalid upgrade payload")
		}
		checksum := p.Checksum
		if p.Name == "knowledge" {
			// 可选外部向量库绑定也是执行载荷的一部分，不能沿用其他配置的成功记录。
			b, err := json.Marshal(struct {
				Enabled   bool
				Knowledge config.KnowledgeSpaceConfig
			}{cfg.FeatureGate.EnableKnowledgeSpace, cfg.KnowledgeSpace})
			if err != nil {
				return nil, "", err
			}
			h := sha256.Sum256(append([]byte(checksum), b...))
			checksum = hex.EncodeToString(h[:])
		}
		step := database.UpgradeStep{ID: "20261011_" + p.Name + "_" + checksum, Checksum: checksum, DependsOn: []string{}, Transactional: p.Name != "knowledge", Recovery: "retry_after_new_plan", Apply: callbacks[p.Name]}
		if len(steps) > 0 {
			step.DependsOn = append(step.DependsOn, steps[len(steps)-1].ID)
		}
		if !step.Transactional {
			step.Recovery = "restore_pre_upgrade_database_config_and_external_vectors"
		}
		steps = append(steps, step)
	}
	h := sha256.Sum256(upgradePayloads)
	return steps, hex.EncodeToString(h[:]), nil
}
