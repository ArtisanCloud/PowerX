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
	pgvectorcfg "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/vectorstore/pgvector"
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
		driver := strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.Driver)
		fmt.Printf("[migrate] knowledge_space enabled=true vector_store.driver=%q db=%s@%s:%d/%s\n",
			driver,
			strings.TrimSpace(cfg.Database.UserName),
			strings.TrimSpace(cfg.Database.Host),
			cfg.Database.Port,
			strings.TrimSpace(cfg.Database.Database),
		)

		if err := migration.EnsureKnowledgeKGAssistTables(db); err != nil {
			return fmt.Errorf("Knowledge Space KG 表迁移失败: %w", err)
		}

		if driver == "pgvector" {
			fmt.Printf("[migrate] pgvector migration target=%s.%s\n",
				coalesce(strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.PgVector.Schema), "public"),
				coalesce(strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.PgVector.Table), "knowledge_vectors_v1_1536"),
			)
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

			pgCfg := pgvectorcfg.Config{
				DSN:              strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.PgVector.DSN),
				Schema:           strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.PgVector.Schema),
				Table:            strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.PgVector.Table),
				Dimensions:       cfg.KnowledgeSpace.VectorStore.PgVector.Dimensions,
				EnableMigrations: cfg.KnowledgeSpace.VectorStore.PgVector.EnableMigrations,
				BatchSize:        cfg.KnowledgeSpace.VectorStore.PgVector.BatchSize,
				Lists:            cfg.KnowledgeSpace.VectorStore.PgVector.Lists,
				TimeoutSeconds:   cfg.KnowledgeSpace.VectorStore.PgVector.TimeoutSeconds,
			}
			if err := migration.EnsureKnowledgeVectorsPGVector(ctx, dsn, pgCfg); err != nil {
				return fmt.Errorf("Knowledge Space pgvector 迁移失败: %w", err)
			}

			// Sanity check for local Postgres: ensure the vectors table is actually visible.
			if strings.EqualFold(strings.TrimSpace(cfg.Database.Driver), "postgres") {
				schema := coalesce(strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.PgVector.Schema), "public")
				table := coalesce(strings.TrimSpace(cfg.KnowledgeSpace.VectorStore.PgVector.Table), "knowledge_vectors_v1_1536")
				var regclass string
				if err := db.WithContext(ctx).Raw(`SELECT to_regclass(?)`, fmt.Sprintf("%s.%s", schema, table)).Scan(&regclass).Error; err != nil {
					return fmt.Errorf("pgvector 表可见性检查失败: %w", err)
				}
				if strings.TrimSpace(regclass) == "" {
					return fmt.Errorf("pgvector 迁移已执行但未发现表：%s.%s（请检查 schema/search_path/权限）", schema, table)
				}
				fmt.Printf("[migrate] pgvector table ready: %s\n", strings.TrimSpace(regclass))
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
