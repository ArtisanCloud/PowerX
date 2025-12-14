package connector_guard

import (
	"context"
	"testing"
	"time"

	"github.com/ArtisanCloud/PowerX/internal/service/agent_model_hub/shared"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent_model_hub"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/crypto"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestVerifyWebhookSignature(t *testing.T) {
	ctx := context.Background()
	db := setupConnectorTestDB(t)
	svc := NewService(Options{
		Options: shared.Options{
			DB:           db,
			Cache:        cache.NewMemoryCache(),
			TenantKeySvc: tenantkeys.NewTenantKeyService(db),
		},
		Instances: repo.NewConnectorInstanceRepository(db),
	})

	inst, err := svc.UpsertInstance(ctx, "default", ConnectorInstanceInput{
		TenantScope: "tenant-demo",
		Platform:    "coze",
		Secrets: map[string]string{
			"oauth_token":         "token-abc",
			"webhook_signing_key": "secret-xyz",
		},
		MappingTemplate: datatypesJSON(t, `{"workflow":"demo"}`),
	})
	require.NoError(t, err)

	payload := []byte(`{"ok":true}`)
	ts := time.Now().UTC().Format(time.RFC3339)
	signature := "sha256=" + computeSignature("secret-xyz", ts, payload)

	err = svc.VerifyWebhookSignature(ctx, WebhookVerificationInput{
		InstanceID: inst.UUID,
		Signature:  signature,
		Timestamp:  ts,
		Payload:    payload,
	})
	require.NoError(t, err)

	err = svc.VerifyWebhookSignature(ctx, WebhookVerificationInput{
		InstanceID: inst.UUID,
		Signature:  "sha256=badsignature",
		Timestamp:  ts,
		Payload:    payload,
	})
	require.ErrorIs(t, err, ErrSignatureMismatch)
}

func TestTrackCallbackMetricAutoPause(t *testing.T) {
	ctx := context.Background()
	db := setupConnectorTestDB(t)
	svc := NewService(Options{
		Options: shared.Options{
			DB:           db,
			Cache:        cache.NewMemoryCache(),
			TenantKeySvc: tenantkeys.NewTenantKeyService(db),
		},
		Instances: repo.NewConnectorInstanceRepository(db),
	})

	inst, err := svc.UpsertInstance(ctx, "default", ConnectorInstanceInput{
		TenantScope: "tenant-demo",
		Platform:    "coze",
		Secrets: map[string]string{
			"oauth_token":         "token-abc",
			"webhook_signing_key": "secret-xyz",
		},
	})
	require.NoError(t, err)

	rate, triggered, err := svc.TrackCallbackMetric(ctx, CallbackMetricInput{
		InstanceID: inst.UUID,
		Success:    false,
		Threshold:  0.1,
	})
	require.NoError(t, err)
	require.True(t, triggered)
	require.GreaterOrEqual(t, rate, 0.1)

	updated, err := svc.repo.FindByUUID(ctx, inst.UUID)
	require.NoError(t, err)
	require.Equal(t, "paused", updated.Status)
	require.NotEmpty(t, updated.LastPauseReason)
}

func setupConnectorTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	require.NoError(t, err)
	coremodel.PowerXSchema = "main"
	require.NoError(t, createConnectorTables(db))
	require.NoError(t, createTenantKeyTables(db))
	require.NoError(t, crypto.SetGlobalKeyB64("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="))
	return db
}

func createConnectorTables(db *gorm.DB) error {
	const connectorSQL = `
CREATE TABLE IF NOT EXISTS main.agent_connector_instances (
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
	sealed_secrets JSON
);`
	if err := db.Exec(connectorSQL).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_connector_instances_uuid ON agent_connector_instances(uuid);`).Error; err != nil {
		return err
	}
	return db.Exec(`CREATE INDEX IF NOT EXISTS idx_connector_instances_scope ON agent_connector_instances(env, tenant_scope);`).Error
}

func createTenantKeyTables(db *gorm.DB) error {
	const tenantSQL = `
CREATE TABLE IF NOT EXISTS main.iam_tenant_key_pairs (
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
);`
	if err := db.Exec(tenantSQL).Error; err != nil {
		return err
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_tenant_keys_scope ON iam_tenant_key_pairs(env, IFNULL(tenant_id, 0));`).Error; err != nil {
		return err
	}
	return db.Exec(`CREATE INDEX IF NOT EXISTS idx_tenant_keys_scope_uuid ON iam_tenant_key_pairs(env, IFNULL(tenant_uuid, ''));`).Error
}

func datatypesJSON(t *testing.T, payload string) datatypes.JSON {
	t.Helper()
	return datatypes.JSON([]byte(payload))
}
