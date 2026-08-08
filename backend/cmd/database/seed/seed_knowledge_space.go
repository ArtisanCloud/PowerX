package seed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/ArtisanCloud/PowerX/config"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore/pgvector"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

type policyTemplateSeed struct {
	TemplateName string
	Version      string
	RAGProfile   map[string]any
	GraphProfile map[string]any
	Masking      map[string]any
	Alerting     map[string]any
}

func SeedKnowledgePolicyTemplates(db *gorm.DB) error {
	items := []policyTemplateSeed{
		{
			TemplateName: "default",
			Version:      "v1",
			RAGProfile:   map[string]any{"profile": "default"},
			GraphProfile: map[string]any{},
			Masking:      map[string]any{},
			Alerting:     map[string]any{},
		},
	}

	now := time.Now().UTC()
	for _, it := range items {
		name := strings.TrimSpace(it.TemplateName)
		ver := strings.TrimSpace(it.Version)
		if name == "" || ver == "" {
			continue
		}

		ragRaw, _ := json.Marshal(it.RAGProfile)
		graphRaw, _ := json.Marshal(it.GraphProfile)
		maskingRaw, _ := json.Marshal(it.Masking)
		alertRaw, _ := json.Marshal(it.Alerting)

		fp := sha256.Sum256([]byte(name + ":" + ver + ":" + string(ragRaw) + ":" + string(graphRaw) + ":" + string(maskingRaw) + ":" + string(alertRaw)))
		hash := hex.EncodeToString(fp[:])

		var existing models.PolicyTemplateVersion
		err := db.WithContext(seedCtx()).
			Where("template_name = ? AND version = ?", name, ver).
			Take(&existing).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}

		if err == nil && existing.ID > 0 {
			if err := db.WithContext(seedCtx()).
				Model(&models.PolicyTemplateVersion{}).
				Where("id = ?", existing.ID).
				Updates(map[string]any{
					"rag_profile":      datatypes.JSON(ragRaw),
					"graph_profile":    datatypes.JSON(graphRaw),
					"masking_profile":  datatypes.JSON(maskingRaw),
					"alerting_profile": datatypes.JSON(alertRaw),
					"immutable_hash":   hash,
					"approved_by":      "seed",
					"approved_at":      &now,
				}).Error; err != nil {
				return err
			}
			logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] policy templates updated: %s-%s (id=%d)", name, ver, existing.ID)
			continue
		}

		row := &models.PolicyTemplateVersion{
			TemplateName:    name,
			Version:         ver,
			RAGProfile:      datatypes.JSON(ragRaw),
			GraphProfile:    datatypes.JSON(graphRaw),
			MaskingProfile:  datatypes.JSON(maskingRaw),
			AlertingProfile: datatypes.JSON(alertRaw),
			ApprovedBy:      "seed",
			ApprovedAt:      &now,
			ImmutableHash:   hash,
		}
		if err := db.WithContext(seedCtx()).Create(row).Error; err != nil {
			return err
		}
		logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] policy templates ready: %s-%s (id=%d)", name, ver, row.ID)
	}
	return nil
}

func SeedBuiltinKnowledgeSpaces(db *gorm.DB, cfg *config.Config, tenantUUID string) error {
	tenantUUID = strings.TrimSpace(tenantUUID)
	if tenantUUID == "" {
		return ksvc.ErrInvalidInput
	}
	pgCfg := buildSeedPGVectorConfig(cfg)
	initializer := ksvc.NewBuiltinKnowledgeInitializer(ksvc.BuiltinKnowledgeInitializerOptions{
		DB:           db,
		SpaceService: ksvc.NewService(ksvc.ServiceOptions{DB: db}),
		VectorIndex: ksvc.NewVectorIndexService(ksvc.VectorIndexServiceOptions{
			DB:       db,
			PGVector: pgCfg,
		}),
	})
	result, err := initializer.EnsureTenantBuiltinKnowledge(seedCtx(), ksvc.BuiltinKnowledgeSeedInput{
		TenantUUID:  tenantUUID,
		RequestedBy: "cmd/database seed",
	})
	if err != nil {
		return err
	}
	logger.InfoF(logger.WithLogFields(context.Background(), map[string]interface{}{"module": "legacy"}), "[seed] builtin knowledge spaces ready: tenant_uuid=%s %s", tenantUUID, ksvc.FormatBuiltinKnowledgeSeedSummary(result))
	return nil
}

func buildSeedPGVectorConfig(cfg *config.Config) pgvector.Config {
	if cfg == nil {
		return pgvector.Config{}
	}
	pgCfg := cfg.KnowledgeSpace.VectorStore.PgVector
	dsn := strings.TrimSpace(pgCfg.DSN)
	if dsn == "" {
		dsn = strings.TrimSpace(cfg.Database.DSN)
	}
	if dsn == "" && strings.TrimSpace(cfg.Database.Host) != "" {
		sslmode := strings.TrimSpace(cfg.Database.SSLMode)
		if sslmode == "" {
			sslmode = "disable"
		}
		tz := strings.TrimSpace(cfg.Database.Timezone)
		if tz == "" {
			tz = "UTC"
		}
		dsn = "host=" + strings.TrimSpace(cfg.Database.Host) +
			" port=" + strconv.Itoa(cfg.Database.Port) +
			" user=" + strings.TrimSpace(cfg.Database.UserName) +
			" password=" + strings.TrimSpace(cfg.Database.Password) +
			" dbname=" + strings.TrimSpace(cfg.Database.Database) +
			" sslmode=" + sslmode +
			" TimeZone=" + tz
	}
	return pgvector.Config{
		DSN:              dsn,
		Schema:           strings.TrimSpace(pgCfg.Schema),
		Table:            strings.TrimSpace(pgCfg.Table),
		Dimensions:       pgCfg.Dimensions,
		EnableMigrations: false,
		BatchSize:        pgCfg.BatchSize,
		Lists:            pgCfg.Lists,
		TimeoutSeconds:   pgCfg.TimeoutSeconds,
	}
}
