package testenv

import (
	"fmt"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/crypto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestEnv provides isolated DB for Agent Model Hub tests.
type TestEnv struct {
	T  *testing.T
	DB *gorm.DB
}

// New creates the sqlite-backed environment and migrates core tables.
func New(t *testing.T) *TestEnv {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	const schema = "main"
	coremodel.PowerXSchema = schema
	if err := createTables(db, schema); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	if err := crypto.SetGlobalKeyB64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="); err != nil {
		t.Fatalf("set global key: %v", err)
	}

	return &TestEnv{
		T:  t,
		DB: db,
	}
}

func createTables(db *gorm.DB, schema string) error {
	providersTable := fmt.Sprintf("%s.%s", schema, "agent_provider_profiles")
	createProviders := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uuid TEXT NOT NULL,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	env TEXT NOT NULL,
	tenant_id INTEGER,
	name TEXT NOT NULL,
	capabilities TEXT DEFAULT '[]',
	primary_endpoint TEXT NOT NULL,
	regions TEXT DEFAULT '[]',
	tenant_whitelist TEXT DEFAULT '[]',
	secret_refs TEXT DEFAULT '{}',
	sealed_secrets TEXT DEFAULT '{}',
	health_score REAL DEFAULT 0,
	rollout_status TEXT DEFAULT 'draft',
	audit_trail_id TEXT,
	metadata TEXT DEFAULT '{}'
);`, providersTable)
	if err := db.Exec(createProviders).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_scope_name ON agent_provider_profiles(env, tenant_id, name);`).Error; err != nil {
		return err
	}

	routingTable := fmt.Sprintf("%s.%s", schema, "agent_routing_policies")
	createRouting := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uuid TEXT NOT NULL,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	env TEXT,
	tenant_scope TEXT,
	version INTEGER,
	status TEXT,
	rules TEXT,
	fallback_chain TEXT,
	approval_record TEXT,
	safe_mode_thresholds TEXT
);`, routingTable)
	if err := db.Exec(createRouting).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_routing_scope_version ON agent_routing_policies(env, tenant_scope, version);`).Error; err != nil {
		return err
	}

	tenantKeysTable := fmt.Sprintf("%s.%s", schema, "iam_tenant_key_pairs")
	createTenantKeys := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	env TEXT NOT NULL,
	tenant_id INTEGER,
	k_id TEXT,
	alg TEXT,
	public_pem TEXT,
	enc_private TEXT DEFAULT '{}',
	active INTEGER DEFAULT 1
);`, tenantKeysTable)
	if err := db.Exec(createTenantKeys).Error; err != nil {
		return err
	}
	if err := db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s.idx_tenant_keys_scope ON iam_tenant_key_pairs(env, IFNULL(tenant_id, 0));`, schema)).Error; err != nil {
		return err
	}
	return nil
}
