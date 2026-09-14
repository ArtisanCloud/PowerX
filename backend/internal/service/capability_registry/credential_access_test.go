package capability_registry

import (
	"context"
	"testing"
	"time"

	capm "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	repo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/capability_registry"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

func TestCredentialAccessRevocationAndTraceOwnership(t *testing.T) {
	db := newGrantStatusTestDB(t)
	require.NoError(t, db.AutoMigrate(&capm.InvocationTrace{}))
	capID := "com.corex.test.read"
	seedGrantStatusCapability(t, db, capID, true)
	row := &setting.PluginInstanceConfig{TenantUUID: grantStatusTestTenant, PluginID: "plugin.one", Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(`{"client_id":"one","allowed_capabilities":["com.corex.test.read"]}`)}
	require.NoError(t, db.Create(row).Error)
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), grantStatusTestTenant), &reqctx.CoreXClaims{TenantUUID: grantStatusTestTenant, PluginID: "plugin.one", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:one", Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}})
	access := NewGrantStatusService(db)
	current, e := access.CurrentAccess(ctx)
	require.NoError(t, e)
	require.NoError(t, current.Require(capID))
	seedGrantStatusCapability(t, db, "com.corex.test.manage", true)
	catalog := NewRegistryService(RegistryServiceOptions{DB: db})
	views, total, e := catalog.ListCapabilities(ctx, CapabilityListOptions{TenantUUID: grantStatusTestTenant, IncludeTotal: true, Limit: 1})
	require.NoError(t, e)
	require.EqualValues(t, 1, total)
	require.Len(t, views, 1)
	require.Equal(t, capID, views[0].Record.CapabilityID)
	trace := "shared-client-trace"
	traces := repo.NewInvocationTraceRepository(db)
	// Same trace ID in a different caller's row must neither leak nor shadow ours.
	_, e = traces.Create(ctx, &capm.InvocationTrace{TraceID: trace, TenantUUID: grantStatusTestTenant, CallerSubject: "sts:plugin.two:client:two", CapabilityID: capID, PluginID: "core", ProtocolUsed: "rest", Status: "completed"})
	require.NoError(t, e)
	_, e = traces.Create(ctx, &capm.InvocationTrace{TraceID: trace, TenantUUID: grantStatusTestTenant, CallerSubject: current.Subject, CapabilityID: capID, PluginID: "core", ProtocolUsed: "rest", Status: "completed"})
	require.NoError(t, e)
	invoker := &InvocationService{catalog: &RegistryService{credentialAccess: access}, traces: traces}
	own, e := invoker.GetTrace(ctx, trace)
	require.NoError(t, e)
	require.Equal(t, current.Subject, own.CallerSubject)
	_, e = traces.Create(ctx, &capm.InvocationTrace{TraceID: "foreign-only", TenantUUID: grantStatusTestTenant, CallerSubject: "sts:plugin.two:client:two", CapabilityID: capID, PluginID: "core", ProtocolUsed: "rest", Status: "completed"})
	require.NoError(t, e)
	_, e = invoker.GetTrace(ctx, "foreign-only")
	require.ErrorIs(t, e, repo.ErrInvocationTraceNotFound)
	require.NoError(t, db.Model(row).Update("value_json", datatypes.JSON(`{"client_id":"one","allowed_capabilities":[]}`)).Error)
	current, e = access.CurrentAccess(ctx)
	require.NoError(t, e)
	require.Error(t, current.Require(capID))
	views, total, e = catalog.ListCapabilities(ctx, CapabilityListOptions{TenantUUID: grantStatusTestTenant, IncludeTotal: true, Limit: 1})
	require.NoError(t, e)
	require.Zero(t, total)
	require.Empty(t, views)
	_, e = invoker.GetTrace(ctx, trace)
	var denied *DirectGrantError
	require.ErrorAs(t, e, &denied)
	require.Equal(t, 403, denied.Status)
	require.NoError(t, db.Model(row).Update("value_json", datatypes.JSON(`{"client_id":"one","allowed_capabilities":["com.corex.test.read"]}`)).Error)
	require.NoError(t, db.Create(&capm.CapabilityRegistration{CapabilityID: capID, TenantUUID: grantStatusTestTenant, ContractRef: "v2", Status: "disabled", Version: 2, RoutingPolicyID: uuid.New()}).Error)
	current, e = access.CurrentAccess(ctx)
	require.NoError(t, e)
	require.False(t, current.Granted[capID])
}
