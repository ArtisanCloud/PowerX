package migration

import (
	"context"
	"fmt"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EnsureKnowledgeVectorsPGVector provisions the pgvector extension + vectors table for Knowledge Space.
// It is designed to be called by `make db-migrate` (via `cmd/database migrate`) and MUST be idempotent.
func EnsureKnowledgeVectorsPGVector(ctx context.Context, dsn string, cfg config.KnowledgeSpaceVectorStorePGVectorConfig) error {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return fmt.Errorf("pgvector migration: dsn is empty")
	}

	schema := strings.TrimSpace(cfg.Schema)
	table := strings.TrimSpace(cfg.Table)
	if schema == "" {
		schema = "public"
	}
	if table == "" {
		table = "knowledge_vectors"
	}
	if cfg.Dimensions <= 0 {
		cfg.Dimensions = 1536
	}
	if cfg.Lists <= 0 {
		cfg.Lists = 100
	}

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("pgvector migration: parse dsn failed: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("pgvector migration: connect failed: %w", err)
	}
	defer pool.Close()

	tableName := fmt.Sprintf("%s.%s", quoteIdent(schema), quoteIdent(table))
	statements := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, quoteIdent(schema)),
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
            space_uuid uuid NOT NULL,
            chunk_uuid uuid NOT NULL,
            embedding vector(%d) NOT NULL,
            metadata jsonb,
            updated_at timestamptz NOT NULL DEFAULT NOW(),
            PRIMARY KEY (space_uuid, chunk_uuid)
        )`, tableName, cfg.Dimensions),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_space_idx ON %s (space_uuid)`, sanitizeIdentifier(table)+"_space_idx", tableName),
		// Note: IVFFLAT requires `vector` extension; keep it optional but idempotent.
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_embedding_idx ON %s USING ivfflat (embedding vector_l2_ops) WITH (lists = %d)`, sanitizeIdentifier(table)+"_embedding_idx", tableName, cfg.Lists),
	}

	for _, stmt := range statements {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			// Provide a clearer message for common pgvector permission/availability issues.
			if strings.Contains(stmt, "CREATE EXTENSION") {
				return fmt.Errorf("pgvector migration: create extension \"vector\" failed (please ensure pgvector is installed and the DB user has CREATE EXTENSION privilege): %w", err)
			}
			return fmt.Errorf("pgvector migration: exec failed: %w (sql=%s)", err, stmt)
		}
	}

	// sanity: ensure extension exists to avoid silent misconfig
	var ok int
	if err := pool.QueryRow(ctx, `SELECT 1 FROM pg_extension WHERE extname = 'vector' LIMIT 1`).Scan(&ok); err != nil || ok != 1 {
		return fmt.Errorf("pgvector migration: extension \"vector\" not found after migration (err=%v)", err)
	}

	return nil
}

// quoteIdent provides a minimal SQL identifier quote (double quotes).
func quoteIdent(s string) string {
	s = strings.ReplaceAll(s, `"`, `""`)
	return `"` + s + `"`
}

// sanitizeIdentifier generates a safe identifier prefix for index names.
// It is NOT used for SQL object addressing (which uses quoteIdent).
func sanitizeIdentifier(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "unnamed"
	}
	// Keep only [a-zA-Z0-9_], replace others with underscore.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	out = strings.Trim(out, "_")
	if out == "" {
		out = "unnamed"
	}
	// Postgres identifier length limit is 63 bytes; keep it short.
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}
