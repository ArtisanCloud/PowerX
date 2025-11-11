package agentmodelhub

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
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
	prevSchema := coremodel.PowerXSchema
	coremodel.PowerXSchema = ""
	t.Cleanup(func() { coremodel.PowerXSchema = prevSchema })

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(
		&modeltenant.TenantKeyPair{},
		&dbmodel.AIProviderCredential{},
		&dbmodel.AIModelProfile{},
	))

	svc := agentSvc.NewAgentSettingService(db)

	ctx := context.Background()
	env := "default"
	tenantID := uint64(42)

	cred := &dbmodel.AIProviderCredential{
		Env:        env,
		TenantID:   &tenantID,
		Name:       "default-openai",
		Provider:   "openai",
		AuthScheme: "bearer",
		Data: datatypes.JSONMap{
			"api_key":  "sk-initial",
			"base_url": "https://api.openai.com/v1",
		},
	}
	prof := &dbmodel.AIModelProfile{
		Env:      env,
		TenantID: &tenantID,
		Modality: "llm",
		Provider: "openai",
		Model:    "gpt-4o-mini",
	}

	require.NoError(t, svc.SaveCredentialAndProfile(ctx, env, &tenantID, cred, prof, true))

	credRepo := repository.NewAIProviderCredentialRepository(db)
	stored, err := credRepo.FindByScopeNameProvider(ctx, env, &tenantID, cred.Name, cred.Provider)
	require.NoError(t, err)
	sealedBefore := toJSON(t, stored.Data["__sealed"])
	require.NotEmpty(t, sealedBefore)

	require.NoError(t, svc.RotateTenantCredentials(ctx, env, &tenantID))

	rotated, err := credRepo.FindByScopeNameProvider(ctx, env, &tenantID, cred.Name, cred.Provider)
	require.NoError(t, err)
	sealedAfter := toJSON(t, rotated.Data["__sealed"])
	require.NotEqual(t, sealedBefore, sealedAfter)
	require.NotContains(t, rotated.Data, "api_key")

	tks := tenantkeys.NewTenantKeyService(db)
	secrets := map[string]string{}
	require.NoError(t, tks.UnsealSensitive(ctx, env, &tenantID, utils.CloneJSONMap(rotated.Data), &secrets))
	require.Equal(t, "sk-initial", secrets["api_key"])
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}
