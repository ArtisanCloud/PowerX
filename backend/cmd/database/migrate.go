// cmd/database/migrate.go
package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/persistence"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/database"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/migration"
	"github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

func MigrateDatabase(ctx context.Context, db *gorm.DB, cfg *config.Config) error {

	if err := database.MigrateCoreModels(db); err != nil {
		return fmt.Errorf("CoreX 迁移失败: %w", err)
	}

	if err := persistence.MigrateAgentModels(db); err != nil {
		return fmt.Errorf("Agent 迁移失败: %w", err)
	}

	// Knowledge Space migrations (tables that are not covered by GORM models).
	// - KG assist tables are cheap and enable `index.kg` readiness.
	// - pgvector is conditional on driver=pgvector.
	if cfg != nil && cfg.FeatureGate.EnableKnowledgeSpace {
		if err := migration.EnsureKnowledgeKGAssistTables(db); err != nil {
			return fmt.Errorf("Knowledge Space KG 表迁移失败: %w", err)
		}

		driver := strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.Driver)
		if driver == "pgvector" {
			dsn := strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.PgVector.DSN)
			if dsn == "" {
				dsn = strings.TrimSpace(cfg.Database.DSN)
			}
			if dsn == "" {
				// Fallback to the same DSN composition logic as `database.Connect`.
				dsn = fmt.Sprintf(
					"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
					cfg.Database.Host,
					cfg.Database.Port,
					cfg.Database.UserName,
					cfg.Database.Password,
					cfg.Database.Database,
					coalesce(cfg.Database.SSLMode, "disable"),
					coalesce(cfg.Database.Timezone, "UTC"),
				)
			}

			if err := migration.EnsureKnowledgeVectorsPGVector(ctx, dsn, cfg.KnowledgeSpace.VectorStore.PgVector); err != nil {
				return fmt.Errorf("Knowledge Space pgvector 迁移失败: %w", err)
			}
		}

		// Postgres-backed sparse/structured/hier readiness (B方案默认) is controlled by explicit backend selection.
		backends := cfg.KnowledgeSpace.IndexBackends
		needsChunkStore := backends.Sparse == "postgres_fts" || backends.StructuredFields == "postgres_jsonb" || backends.Hier == "postgres_links"
		if needsChunkStore {
			if err := migration.EnsureKnowledgeChunkStoreTables(db); err != nil {
				return fmt.Errorf("Knowledge Space chunk store 迁移失败: %w", err)
			}
		}
		if backends.Hier == "postgres_links" {
			if err := migration.EnsureKnowledgeChunkLinkTables(db); err != nil {
				return fmt.Errorf("Knowledge Space chunk links 迁移失败: %w", err)
			}
		}
	}

	return nil
}

func ResetDatabase(ctx context.Context, db *gorm.DB) error {
	// 如果你用 GORM，可以直接 drop 所有表
	// 或者先获取表名，再循环 drop
	// 这里举例简单版本：
	err := db.Exec("DROP SCHEMA IF EXISTS " + model.PowerXSchema + " CASCADE; CREATE SCHEMA " + model.PowerXSchema + ";").Error
	if err != nil {
		return err
	}
	return nil
}

func coalesce(v string, fallback string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return fallback
	}
	return v
}
