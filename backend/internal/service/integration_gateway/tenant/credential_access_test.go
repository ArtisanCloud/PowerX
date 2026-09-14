package tenant

import (
	"context"
	"testing"
	"time"

	capaccess "github.com/ArtisanCloud/PowerX/internal/service/capability_registry"
	router "github.com/ArtisanCloud/PowerX/internal/service/capability_registry/router"
	core "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	capm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/integration_gateway"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRoutesFilterCurrentCredentialAndDenyRevokedInvoke(t *testing.T) {
	previous := core.PowerXSchema
	core.PowerXSchema = "main"
	t.Cleanup(func() { core.PowerXSchema = previous })
	db, e := gorm.Open(sqlite.Open("file:gateway_access_"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, e)
	raw, e := db.DB()
	require.NoError(t, e)
	t.Cleanup(func() { raw.Close() })
	require.NoError(t, db.AutoMigrate(&m.IntegrationRoute{}, &m.IntegrationInvocationLog{}, &capm.CapabilityRecord{}, &capm.CapabilityRegistration{}, &setting.PluginInstanceConfig{}))
	tenant := uuid.NewString()
	capID := "com.corex.test.read"
	for _, id := range []string{capID, "com.corex.test.manage"} {
		require.NoError(t, db.Create(&capm.CapabilityRecord{CapabilityID: id, PluginID: "core", PluginVersion: "v1", Status: "published"}).Error)
		require.NoError(t, db.Create(&capm.CapabilityRegistration{CapabilityID: id, TenantUUID: tenant, ContractRef: "v1", Status: "published", Version: 1, RoutingPolicyID: uuid.New()}).Error)
		require.NoError(t, db.Create(&m.IntegrationRoute{TenantUUID: tenant, RouteSlug: id, CapabilityID: id, LifecycleState: "active", Status: "enabled"}).Error)
	}
	row := &setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: "plugin.test", Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(`{"client_id":"test","allowed_capabilities":["com.corex.test.read"]}`)}
	require.NoError(t, db.Create(row).Error)
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant), &reqctx.CoreXClaims{TenantUUID: tenant, PluginID: "plugin.test", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:test", Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	executor := &credentialRouter{payload: []byte(`{"value":true}`)}
	service := NewService(ServiceOptions{DB: db, Router: executor})
	routes, e := service.ListRoutes(ctx, tenant, "", "")
	require.NoError(t, e)
	require.Len(t, routes, 1)
	require.Equal(t, capID, routes[0].CapabilityID)
	_, e = service.GetRoute(ctx, tenant, capID)
	require.NoError(t, e)
	_, e = service.GetRoute(ctx, tenant, "com.corex.test.manage")
	var hidden ErrRouteNotAccessible
	require.ErrorAs(t, e, &hidden)
	_, e = service.GetRoute(ctx, uuid.NewString(), capID)
	require.ErrorAs(t, e, &hidden)
	result, e := service.Invoke(ctx, InvokeInput{TenantUUID: tenant, RouteSlug: capID, Channel: "http", Payload: map[string]any{}})
	require.NoError(t, e)
	require.Equal(t, InvokeStatusOK, result.Status)
	require.Equal(t, capID, executor.input.CapabilityID)
	require.Equal(t, tenant, executor.input.TenantUUID)
	require.Equal(t, true, result.Result["value"])
	executor.payload = []byte(`not_json`)
	result, e = service.Invoke(ctx, InvokeInput{TenantUUID: tenant, RouteSlug: capID, Channel: "http"})
	require.Error(t, e)
	require.Equal(t, InvokeStatusFailed, result.Status)
	require.Nil(t, result.Result)
	callsBeforeRevoke := executor.calls
	require.NoError(t, db.Model(row).Update("value_json", datatypes.JSON(`{"client_id":"test","allowed_capabilities":[]}`)).Error)
	routes, e = service.ListRoutes(ctx, tenant, "", "")
	require.NoError(t, e)
	require.Empty(t, routes)
	_, e = service.Invoke(ctx, InvokeInput{TenantUUID: tenant, RouteSlug: capID, Channel: "http"})
	var denied *capaccess.DirectGrantError
	require.ErrorAs(t, e, &denied)
	require.Equal(t, 403, denied.Status)
	require.Equal(t, callsBeforeRevoke, executor.calls)
	require.NoError(t, db.Migrator().DropTable(&m.IntegrationRoute{}))
	_, e = service.GetRoute(ctx, tenant, capID)
	require.ErrorAs(t, e, &denied)
	require.Equal(t, 503, denied.Status)
}

type credentialRouter struct {
	calls   int
	input   router.InvokeRequest
	payload []byte
}

func (r *credentialRouter) Invoke(_ context.Context, in router.InvokeRequest) (router.InvokeResult, error) {
	r.calls++
	r.input = in
	return router.InvokeResult{Payload: r.payload}, nil
}
