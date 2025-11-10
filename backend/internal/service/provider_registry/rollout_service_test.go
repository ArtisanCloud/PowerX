package provider_registry

import (
	"context"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent_model_hub"
	providerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/agent_model_hub"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestScheduleRollout(t *testing.T) {
	db := newTestDB(t)
	svc := testServiceWithDB(db)
	providerID := seedProvider(t, db, "default")

	profile, err := svc.ScheduleRollout(context.Background(), providerID, RolloutPlanInput{
		Env:        "default",
		Strategy:   "canary",
		Percentage: 20,
		Tenants: []TenantRef{
			{TenantID: "demo", Environment: "staging"},
		},
		Note: "initial gray release",
	})
	require.NoError(t, err)
	require.Equal(t, "gray", profile.RolloutStatus)
	require.Contains(t, profile.Metadata, "rollout_plan")

	var stored model.ProviderProfile
	require.NoError(t, db.First(&stored, "uuid = ?", providerID).Error)
	require.Equal(t, "gray", stored.RolloutStatus)
	require.Contains(t, stored.Metadata, "rollout_plan")
}

func TestRollbackProvider(t *testing.T) {
	db := newTestDB(t)
	svc := testServiceWithDB(db)
	providerID := seedProvider(t, db, "default")

	_, err := svc.ScheduleRollout(context.Background(), providerID, RolloutPlanInput{
		Env:        "default",
		Percentage: 10,
		Tenants: []TenantRef{
			{TenantID: "demo", Environment: "staging"},
		},
	})
	require.NoError(t, err)

	profile, err := svc.RollbackProvider(context.Background(), providerID, RollbackInput{
		Env:    "default",
		Reason: "validation alert",
	})
	require.NoError(t, err)
	require.Equal(t, "rolled_back", profile.RolloutStatus)

	var stored model.ProviderProfile
	require.NoError(t, db.First(&stored, "uuid = ?", providerID).Error)
	plan := cloneJSONMap(stored.Metadata["rollout_plan"])
	require.Equal(t, "validation alert", getString(plan["rollback_reason"]))
}

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	coremodel.PowerXSchema = ""
	create := `
CREATE TABLE IF NOT EXISTS agent_provider_profiles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uuid TEXT NOT NULL,
	created_at DATETIME,
	updated_at DATETIME,
	deleted_at DATETIME,
	env TEXT,
	tenant_id INTEGER,
	name TEXT,
	capabilities TEXT,
	primary_endpoint TEXT,
	regions TEXT,
	tenant_whitelist TEXT,
	secret_refs TEXT,
	sealed_secrets TEXT,
	health_score REAL,
	rollout_status TEXT,
	audit_trail_id TEXT,
	metadata TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_provider_uuid ON agent_provider_profiles(uuid);`
	require.NoError(t, db.Exec(create).Error)
	return db
}

func testServiceWithDB(db *gorm.DB) *Service {
	return &Service{
		db:    db,
		repo:  providerrepo.NewProviderProfileRepository(db),
		clock: func() time.Time { return time.Unix(0, 0).UTC() },
	}
}

func seedProvider(t *testing.T, db *gorm.DB, env string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	rec := &model.ProviderProfile{
		Env:           env,
		Name:          "seed-provider",
		RolloutStatus: "draft",
		Metadata:      datatypes.JSONMap{},
	}
	rec.UUID = id
	require.NoError(t, db.Create(rec).Error)
	return id
}
