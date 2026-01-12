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

func TestDBMigrateChunkStoreEnabledCreatesTables(t *testing.T) {
	dsn := os.Getenv("POWERX_TEST_PG_DSN")
	if dsn == "" {
		dsn = os.Getenv("POWERX_TEST_PGVECTOR_DSN")
	}
	if dsn == "" {
		t.Skip("POWERX_TEST_PG_DSN or POWERX_TEST_PGVECTOR_DSN is not set; skipping db-migrate chunk store verification")
	}

	backendDir := filepath.Join(findRepoRoot(t), "backend")
	cfgPath := writeTempChunkConfig(t, backendDir, dsn, true)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	runMigrate := func() {
		cmd := exec.CommandContext(ctx, "go", "run", "./cmd/database", "migrate", "--config", cfgPath)
		cmd.Dir = backendDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "migrate failed: %s", string(out))
	}

	runMigrate()
	runMigrate() // idempotency

	pool := openPool(t, ctx, dsn)
	assertRegclass(t, ctx, pool, "public.knowledge_chunks")
	assertRegclass(t, ctx, pool, "public.knowledge_chunk_links")
}

func TestDBMigrateChunkStoreDisabledSkipsTables(t *testing.T) {
	dsn := os.Getenv("POWERX_TEST_PG_DSN")
	if dsn == "" {
		dsn = os.Getenv("POWERX_TEST_PGVECTOR_DSN")
	}
	if dsn == "" {
		t.Skip("POWERX_TEST_PG_DSN or POWERX_TEST_PGVECTOR_DSN is not set; skipping db-migrate chunk store verification")
	}

	backendDir := filepath.Join(findRepoRoot(t), "backend")
	cfgPath := writeTempChunkConfig(t, backendDir, dsn, false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	runMigrate := func() {
		cmd := exec.CommandContext(ctx, "go", "run", "./cmd/database", "migrate", "--config", cfgPath)
		cmd.Dir = backendDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "migrate failed: %s", string(out))
	}

	runMigrate()
	runMigrate() // idempotency

	pool := openPool(t, ctx, dsn)
	assertRegclassNil(t, ctx, pool, "public.knowledge_chunks")
	assertRegclassNil(t, ctx, pool, "public.knowledge_chunk_links")
}

func writeTempChunkConfig(t *testing.T, backendDir string, dsn string, enableChunkStore bool) string {
	t.Helper()

	sparse := "external"
	hier := "external"
	structured := "external"
	if enableChunkStore {
		sparse = "postgres_fts"
		hier = "postgres_links"
		structured = "postgres_jsonb"
	}

	content := []byte(`
feature_gate:
  enable_knowledge_space: true

database:
  driver: postgres
  dsn: "` + escapeYAMLString(dsn) + `"

knowledge_space:
  vector_store:
    driver: "" # avoid pgvector requirement for this test
  index_backends:
    sparse: ` + sparse + `
    hier: ` + hier + `
    structured_fields: ` + structured + `
    kg: postgres
`)

	tmpDir := filepath.Join(backendDir, "tmp", "test-configs")
	require.NoError(t, os.MkdirAll(tmpDir, 0o755))
	cfgPath := filepath.Join(tmpDir, "db-migrate-chunk-store-"+time.Now().Format("20060102150405.000000000")+".yaml")
	require.NoError(t, os.WriteFile(cfgPath, content, 0o644))
	t.Cleanup(func() { _ = os.Remove(cfgPath) })
	return cfgPath
}

func openPool(t *testing.T, ctx context.Context, dsn string) *pgxpool.Pool {
	t.Helper()
	poolCfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func assertRegclass(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) {
	t.Helper()
	var got *string
	err := pool.QueryRow(ctx, `select to_regclass($1)`, name).Scan(&got)
	require.NoError(t, err)
	require.NotNil(t, got, "expected table to exist: %s", name)
}

func assertRegclassNil(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) {
	t.Helper()
	var got *string
	err := pool.QueryRow(ctx, `select to_regclass($1)`, name).Scan(&got)
	require.NoError(t, err)
	require.Nil(t, got, "expected table to be absent: %s", name)
}
