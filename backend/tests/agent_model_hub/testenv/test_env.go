package testenv

import (
	"fmt"
	"testing"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	"github.com/ArtisanCloud/PowerX/pkg/crypto"
	"github.com/google/uuid"
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

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
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
	tenantTable := fmt.Sprintf("%s.%s", schema, coremodel.TableIAMTenant)
	createTenants := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uuid TEXT NOT NULL UNIQUE,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	key TEXT,
	name TEXT,
	status INTEGER DEFAULT 1,
	type TEXT,
	plan TEXT,
	domain TEXT,
	description TEXT
);`, tenantTable)
	if err := db.Exec(createTenants).Error; err != nil {
		return err
	}
	if err := db.Exec(fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS %s.idx_tenant_uuid ON %s(uuid);`, schema, coremodel.TableIAMTenant)).Error; err != nil {
		return err
	}

	providersTable := fmt.Sprintf("%s.%s", schema, "agent_provider_profiles")
	createProviders := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uuid TEXT NOT NULL UNIQUE,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	env TEXT NOT NULL,
	tenant_uuid TEXT DEFAULT '',
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
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_provider_scope_name ON agent_provider_profiles(env, tenant_uuid, name);`).Error; err != nil {
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

	connectorTable := fmt.Sprintf("%s.%s", schema, "agent_connector_instances")
	createConnectors := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uuid TEXT NOT NULL,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	env TEXT NOT NULL,
	tenant_scope TEXT NOT NULL,
	platform TEXT NOT NULL,
	region TEXT,
	oauth_ref TEXT NOT NULL,
	webhook_signing_key_ref TEXT NOT NULL,
	mapping_template TEXT DEFAULT '{}',
	status TEXT DEFAULT 'active',
	error_rate REAL DEFAULT 0,
	last_pause_reason TEXT,
	rate_limit_per_minute INTEGER DEFAULT 0,
	sealed_secrets TEXT DEFAULT '{}'
);`, connectorTable)
	if err := db.Exec(createConnectors).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_connector_instances_uuid ON agent_connector_instances(uuid);`).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_connector_instances_scope ON agent_connector_instances(env, tenant_scope);`).Error; err != nil {
		return err
	}

	costLedgerTable := fmt.Sprintf("%s.%s", schema, "agent_cost_quota_ledgers")
	createCostLedgers := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uuid TEXT NOT NULL,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	env TEXT NOT NULL,
	tenant_uuid TEXT NOT NULL,
	budget_period TEXT NOT NULL,
	provider_profile_id TEXT,
	quota_limit REAL DEFAULT 0,
	usage_actual REAL DEFAULT 0,
	anomaly_state TEXT DEFAULT '{}',
	enforcement_state TEXT DEFAULT '{}',
	sealed_metadata TEXT DEFAULT '{}',
	dashboard_scope TEXT DEFAULT '',
	last_anomaly_at DATETIME
);`, costLedgerTable)
	if err := db.Exec(createCostLedgers).Error; err != nil {
		return err
	}
	if err := db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s.idx_cost_quota_scope ON agent_cost_quota_ledgers(env, tenant_uuid);`, schema)).Error; err != nil {
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
	tenant_uuid TEXT,
	k_id TEXT,
	alg TEXT,
	public_pem TEXT,
	enc_private TEXT DEFAULT '{}',
	active INTEGER DEFAULT 1
);`, tenantKeysTable)
	if err := db.Exec(createTenantKeys).Error; err != nil {
		return err
	}
	if err := db.Exec(fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s.idx_tenant_keys_scope ON iam_tenant_key_pairs(env, tenant_uuid);`, schema)).Error; err != nil {
		return err
	}
	return nil
}

// MustInsertTenant ensures sqlite fixtures include a tenant row for UUID-only tests.
func (e *TestEnv) MustInsertTenant(id uint64, tenantUUID string) {
	e.T.Helper()
	if tenantUUID == "" {
		tenantUUID = uuid.NewString()
	}
	key := fmt.Sprintf("tenant-%d", id)
	name := fmt.Sprintf("Tenant %d", id)
	stmt := fmt.Sprintf(`INSERT INTO %s.%s (id, uuid, created_at, updated_at, key, name, status, type, plan)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?, 1, 'enterprise', 'pro');`, coremodel.PowerXSchema, coremodel.TableIAMTenant)
	if err := e.DB.Exec(stmt, id, tenantUUID, key, name).Error; err != nil {
		e.T.Fatalf("insert tenant %s: %v", tenantUUID, err)
	}
}
