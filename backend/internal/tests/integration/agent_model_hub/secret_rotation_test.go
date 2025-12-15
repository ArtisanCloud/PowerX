package agentmodelhub

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	dbmodel "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/model"
	repository "github.com/ArtisanCloud/PowerX/internal/server/agent/persistence/repository"
	agentSvc "github.com/ArtisanCloud/PowerX/internal/service/agent"
	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modeltenant "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/utils"
)

func TestSecretRotationResealsCredentials(t *testing.T) {
	if os.Getenv("AGENT_MODEL_HUB_TEST_DSN") == "" {
		t.Skip("AGENT_MODEL_HUB_TEST_DSN not set; skipping agent model hub credential rotation test")
	}
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	require.NoError(t, db.Exec(`
CREATE TABLE IF NOT EXISTS main.iam_tenant_key_pairs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    env TEXT,
    tenant_id INTEGER,
    kid TEXT,
    alg TEXT,
    public_pem TEXT,
    enc_private TEXT,
    active BOOLEAN DEFAULT 1
)`).Error)

	require.NoError(t, db.AutoMigrate(
		&modeltenant.Tenant{},
		&dbmodel.AIProviderCredential{},
		&dbmodel.AIModelProfile{},
	))

	tenant := &modeltenant.Tenant{
		Key:    "test-system",
		Name:   "Test System",
		Type:   modeltenant.TenantTypeSystem,
		Plan:   modeltenant.TenantPlanFree,
		Status: modeltenant.TenantStatusActive,
	}
	require.NoError(t, db.Create(tenant).Error)

	tenantUUID := tenant.UUID.String()
	tenantUUIDPtr := tenantUUID
	svc := agentSvc.NewAgentSettingService(db)

	ctx := context.Background()
	env := "default"

	cred := &dbmodel.AIProviderCredential{
		Env:        env,
		TenantUUID: &tenantUUIDPtr,
		Name:       "default-openai",
		Provider:   "openai",
		AuthScheme: "bearer",
		Data: datatypes.JSONMap{
			"api_key":  "sk-initial",
			"base_url": "https://api.openai.com/v1",
		},
	}
	prof := &dbmodel.AIModelProfile{
		Env:        env,
		TenantUUID: &tenantUUIDPtr,
		Modality:   "llm",
		Provider:   "openai",
		Model:      "gpt-4o-mini",
	}

	require.NoError(t, svc.SaveCredentialAndProfile(ctx, env, &tenantUUIDPtr, cred, prof, true))

	credRepo := repository.NewAIProviderCredentialRepository(db)
	stored, err := credRepo.FindByScopeNameProvider(ctx, env, &tenantUUIDPtr, cred.Name, cred.Provider)
	require.NoError(t, err)
	sealedBefore := toJSON(t, stored.Data["__sealed"])
	require.NotEmpty(t, sealedBefore)

	require.NoError(t, svc.RotateTenantCredentials(ctx, env, &tenantUUIDPtr))

	rotated, err := credRepo.FindByScopeNameProvider(ctx, env, &tenantUUIDPtr, cred.Name, cred.Provider)
	require.NoError(t, err)
	sealedAfter := toJSON(t, rotated.Data["__sealed"])
	require.NotEqual(t, sealedBefore, sealedAfter)
	require.NotContains(t, rotated.Data, "api_key")

	tks := tenantkeys.NewTenantKeyService(db)
	secrets := map[string]string{}
	require.NoError(t, tks.UnsealSensitive(ctx, env, tenantUUIDPtr, utils.CloneJSONMap(rotated.Data), &secrets))
	require.Equal(t, "sk-initial", secrets["api_key"])
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
