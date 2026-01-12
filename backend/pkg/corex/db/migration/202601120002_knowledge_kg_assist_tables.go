package migration

import (
	"fmt"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"gorm.io/gorm"
)

// EnsureKnowledgeKGAssistTables provisions minimal KG assist tables for Knowledge Space.
// This is intentionally small (nodes/edges) and MUST be idempotent.
func EnsureKnowledgeKGAssistTables(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	schema := coremodel.PowerXSchema
	if schema == "" {
		schema = "public"
	}

	nodesTable := fmt.Sprintf("%s.%s", quoteIdent(schema), quoteIdent("knowledge_kg_nodes"))
	edgesTable := fmt.Sprintf("%s.%s", quoteIdent(schema), quoteIdent("knowledge_kg_edges"))

	stmts := []string{
		fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, quoteIdent(schema)),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
  space_uuid uuid NOT NULL,
  node_id text NOT NULL,
  node_type text NOT NULL DEFAULT 'entity',
  props jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (space_uuid, node_id)
)`, nodesTable),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
  space_uuid uuid NOT NULL,
  edge_id text NOT NULL,
  src_node_id text NOT NULL,
  dst_node_id text NOT NULL,
  predicate text NOT NULL,
  props jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (space_uuid, edge_id)
)`, edgesTable),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS knowledge_kg_edges_space_idx ON %s (space_uuid)`, edgesTable),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS knowledge_kg_edges_src_idx ON %s (space_uuid, src_node_id)`, edgesTable),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS knowledge_kg_edges_dst_idx ON %s (space_uuid, dst_node_id)`, edgesTable),
	}

	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}
