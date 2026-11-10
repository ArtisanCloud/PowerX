package authgrpc

import (
	"context"
	"testing"

	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/internal/service/setting"
	"github.com/ArtisanCloud/PowerX/pkg/auth"
	"github.com/ArtisanCloud/PowerX/pkg/cache"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	sm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	tm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/tenant"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/stsproof"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestExchangeRecordsAuthenticatedSTSIdentity(t *testing.T) {
	old := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = old })
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&tm.Tenant{}, &sm.PluginInstanceConfig{}))
	tenant := tm.Tenant{Key: uuid.NewString(), Name: "STS test", Status: 1}
	require.NoError(t, db.Create(&tenant).Error)
	deps := &shared.Deps{DB: db}
	clientID, secret, err := setting.NewPluginInstanceConfigService(deps).EnsureCredentials(context.Background(), tenant.UUID.String(), "com.example.issuer", &setting.ClientCredential{AllowedAudiences: []string{"powerx:api"}, AllowedScopes: []string{"access"}})
	require.NoError(t, err)
	key := []byte("fixture-STS-signing-key")
	ring := NewKeyRing()
	ring.AddHS("default", key)
	svc := NewSTSServiceServerWithRing(deps, ring)
	svc.issuanceStore = cache.NewMemoryCache()
	request := &stsv1.ExchangeRequest{ClientId: clientID, ClientSecret: secret, Audience: "powerx:api", Scope: "access", TtlSeconds: 300}
	result, err := svc.Exchange(context.Background(), request)
	require.NoError(t, err)
	require.EqualValues(t, 200, result.Meta.Code)
	claims, err := auth.ParseAndValidate(result.Data.AccessToken, key, "powerx-sts", "powerx:api")
	require.NoError(t, err)
	require.NoError(t, stsproof.Verify(context.Background(), svc.issuanceStore, claims))
	require.NotEmpty(t, claims.ID)
	request.ClientSecret = "incorrect"
	denied, err := svc.Exchange(context.Background(), request)
	require.NoError(t, err)
	require.EqualValues(t, 401, denied.Meta.Code)
	require.Nil(t, denied.Data)
}
