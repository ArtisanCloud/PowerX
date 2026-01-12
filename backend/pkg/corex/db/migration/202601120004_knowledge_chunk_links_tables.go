package migration

import (
	"fmt"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureKnowledgeChunkLinkTables provisions a minimal adjacency/relationship table
// for hierarchical indices and context expansion.
// This MUST be idempotent.
func EnsureKnowledgeChunkLinkTables(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	schema := coremodel.PowerXSchema
	if schema == "" {
		schema = "public"
	}

	linksTable := fmt.Sprintf("%s.%s", quoteIdent(schema), quoteIdent("knowledge_chunk_links"))

	stmts := []string{
		fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, quoteIdent(schema)),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
  space_uuid uuid NOT NULL,
  src_chunk_uuid uuid NOT NULL,
  dst_chunk_uuid uuid NOT NULL,
  rel_type text NOT NULL,
  props jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (space_uuid, src_chunk_uuid, dst_chunk_uuid, rel_type)
)`, linksTable),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS knowledge_chunk_links_space_src_idx ON %s (space_uuid, src_chunk_uuid)`, linksTable),
	}

	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}
