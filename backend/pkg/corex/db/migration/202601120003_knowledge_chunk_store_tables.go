package migration

import (
	"fmt"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureKnowledgeChunkStoreTables provisions a Postgres-backed chunk store to support:
// - `index.sparse` via Postgres FTS
// - `index.structured_fields` via jsonb filtering
// - `index.hier` via kind + optional links table
//
// This MUST be idempotent.
func EnsureKnowledgeChunkStoreTables(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	schema := coremodel.PowerXSchema
	if schema == "" {
		schema = "public"
	}

	chunksTable := fmt.Sprintf("%s.%s", quoteIdent(schema), quoteIdent("knowledge_chunks"))

	stmts := []string{
		fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, quoteIdent(schema)),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
  space_uuid uuid NOT NULL,
  chunk_uuid uuid NOT NULL,
  job_uuid uuid,
  kind text NOT NULL DEFAULT 'chunk',
  content text NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (space_uuid, chunk_uuid)
)`, chunksTable),
		// Backfill / upgrade for existing installs.
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN IF NOT EXISTS job_uuid uuid`, chunksTable),
		// Postgres FTS: using 'simple' config keeps multilingual content usable without custom dictionaries.
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS knowledge_chunks_fts_idx ON %s USING gin (to_tsvector('simple', content))`, chunksTable),
		// Structured filtering: jsonb contains/path ops.
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS knowledge_chunks_meta_idx ON %s USING gin (metadata jsonb_path_ops)`, chunksTable),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS knowledge_chunks_kind_idx ON %s (space_uuid, kind)`, chunksTable),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS knowledge_chunks_job_idx ON %s (space_uuid, job_uuid)`, chunksTable),
	}

	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}
