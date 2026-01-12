package knowledge_space_integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestDBMigrateProvisionsVectorAndKGTables(t *testing.T) {
	dsn := os.Getenv("POWERX_TEST_PGVECTOR_DSN")
	if dsn == "" {
		t.Skip("POWERX_TEST_PGVECTOR_DSN is not set; skipping pgvector migration verification")
	}

	backendDir := filepath.Join(findRepoRoot(t), "backend")

	cfgPath := writeTempConfig(t, backendDir, dsn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	runMigrate := func() {
		cmd := exec.CommandContext(ctx, "go", "run", "./cmd/database", "migrate", "--config", cfgPath)
		cmd.Dir = backendDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "migrate failed: %s", string(out))
	}

	// 1st run: create
	runMigrate()
	// 2nd run: idempotency
	runMigrate()

	poolCfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	assertRegclass := func(name string) {
		var got *string
		err := pool.QueryRow(ctx, `select to_regclass($1)`, name).Scan(&got)
		require.NoError(t, err)
		require.NotNil(t, got, "expected table to exist: %s", name)
	}

	assertRegclass("public.knowledge_vectors")
	assertRegclass("public.knowledge_kg_nodes")
	assertRegclass("public.knowledge_kg_edges")
}

func writeTempConfig(t *testing.T, backendDir string, dsn string) string {
	t.Helper()

	// Minimal config overlay: use defaults + override DB DSN + enable knowledge space + select pgvector driver.
	content := []byte(`
feature_gate:
  enable_knowledge_space: true

database:
  driver: postgres
  dsn: "` + escapeYAMLString(dsn) + `"

knowledge_space:
  vector_store:
    driver: pgvector
    pgvector:
      dsn: "` + escapeYAMLString(dsn) + `"
      schema: public
      table: knowledge_vectors
      dimensions: 1536
      ivfflat_lists: 10
`)

	tmpDir := filepath.Join(backendDir, "tmp", "test-configs")
	require.NoError(t, os.MkdirAll(tmpDir, 0o755))
	cfgPath := filepath.Join(tmpDir, "db-migrate-pgvector-"+time.Now().Format("20060102150405.000000000")+".yaml")
	require.NoError(t, os.WriteFile(cfgPath, content, 0o644))
	t.Cleanup(func() { _ = os.Remove(cfgPath) })
	return cfgPath
}

func escapeYAMLString(s string) string {
	// naive but sufficient for DSN strings used in tests
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '"' {
			out = append(out, '\\', '"')
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("repo root not found from %s", dir)
		}
		dir = parent
	}
}
