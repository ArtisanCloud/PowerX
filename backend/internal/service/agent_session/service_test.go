package agent_session

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	m "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/agent"
	capmodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/capability_registry"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func fixture(t *testing.T) (*Service, *gorm.DB, context.Context, uuid.UUID) {
	t.Helper()
	previous := coremodel.PowerXSchema
	coremodel.PowerXSchema = "main"
	t.Cleanup(func() { coremodel.PowerXSchema = previous })
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	for i := 0; i < 2; i++ {
		require.NoError(t, db.AutoMigrate(&m.ServiceSession{}, &m.ServiceMessage{}, &m.ServiceInvocation{}, &capmodel.CapabilityRecord{}, &capmodel.CapabilityRegistration{}, &setting.PluginInstanceConfig{}))
	}
	require.NoError(t, db.Exec(`CREATE TABLE agents (id integer primary key, uuid text, tenant_uuid text, owner_plugin_id text, status text, deleted_at datetime)`).Error)
	tenant, agent := uuid.New(), uuid.New()
	require.NoError(t, db.Exec("INSERT INTO agents (uuid,tenant_uuid,owner_plugin_id,status) VALUES (?,?,?,?)", agent, tenant, "plugin.test", "active").Error)
	require.NoError(t, db.Create(&capmodel.CapabilityRecord{CapabilityID: SessionCapability, PluginID: "core", PluginVersion: "1", Title: "test", CapabilitiesHash: "hash", ProtocolHash: "hash", Status: "published"}).Error)
	for _, p := range []string{"plugin.test", "plugin.other"} {
		raw, _ := json.Marshal(map[string]any{"client_id": p, "allowed_capabilities": []string{SessionCapability}})
		require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: tenant.String(), PluginID: p, Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON(raw)}).Error)
	}
	require.NoError(t, db.Create(&capmodel.CapabilityRegistration{TenantUUID: tenant.String(), CapabilityID: SessionCapability, Status: "published", ContractRef: "test", Version: 1}).Error)
	ctx := reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenant.String()), &reqctx.CoreXClaims{TenantUUID: tenant.String(), PluginID: "plugin.test", RegisteredClaims: jwt.RegisteredClaims{Issuer: "powerx-sts", Subject: "client:plugin.test", Audience: jwt.ClaimStrings{"powerx:api"}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(48 * time.Hour))}})
	return NewService(db), db, ctx, agent
}

func TestSessionLifecycleMessagesAndIsolation(t *testing.T) {
	svc, db, ctx, agent := fixture(t)
	created, err := svc.Create(ctx, agent, "test")
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, created.SessionUUID)
	m1, err := svc.Append(ctx, created.SessionUUID, "key-1", "user", "content")
	require.NoError(t, err)
	repeated, err := svc.Append(ctx, created.SessionUUID, "key-1", "user", "content")
	require.NoError(t, err)
	require.Equal(t, m1.MessageUUID, repeated.MessageUUID)
	_, err = svc.Append(ctx, created.SessionUUID, "key-1", "user", "different")
	require.ErrorIs(t, err, ErrConflict)
	for _, role := range []string{"system", "assistant", "tool"} {
		_, err = svc.Append(ctx, created.SessionUUID, "key-2", role, "content")
		require.ErrorIs(t, err, ErrInvalid)
	}
	list, total, err := svc.Messages(ctx, created.SessionUUID, 1, 10)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.EqualValues(t, 1, total)
	otherClaims := *reqctx.GetClaims(ctx)
	otherClaims.PluginID = "plugin.other"
	otherClaims.Subject = "client:plugin.other"
	other := reqctx.WithClaims(ctx, &otherClaims)
	_, err = svc.Get(other, created.SessionUUID)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = svc.Append(other, created.SessionUUID, "key-3", "user", "content")
	require.ErrorIs(t, err, ErrNotFound)
	_, err = svc.Create(other, agent, "test")
	require.ErrorIs(t, err, ErrForbidden)
	otherTenant := uuid.NewString()
	require.NoError(t, db.Create(&capmodel.CapabilityRegistration{TenantUUID: otherTenant, CapabilityID: SessionCapability, Status: "published", ContractRef: "test", Version: 1}).Error)
	require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: otherTenant, PluginID: "plugin.test", Key: "auth.credentials", Enabled: true, ValueJSON: datatypes.JSON([]byte(`{"client_id":"plugin.test","allowed_capabilities":["com.corex.agent.session.manage"]}`))}).Error)
	otherClaims = *reqctx.GetClaims(ctx)
	otherClaims.TenantUUID = otherTenant
	crossTenant := reqctx.WithClaims(reqctx.WithTenantUUID(ctx, otherTenant), &otherClaims)
	_, err = svc.Get(crossTenant, created.SessionUUID)
	require.ErrorIs(t, err, ErrNotFound)
	raw, err := json.Marshal(created)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	for _, forbidden := range []string{"id", "agent_id", "userId", "agentId", "plugin_id", "service_actor"} {
		require.NotContains(t, fields, forbidden)
	}
	_, err = svc.Mutate(ctx, created.SessionUUID, "archive", "")
	require.NoError(t, err)
	_, err = svc.Append(ctx, created.SessionUUID, "key-3", "user", "content")
	require.ErrorIs(t, err, ErrConflict)
	for i := 0; i < 2; i++ {
		_, err = svc.Mutate(ctx, created.SessionUUID, "delete", "")
		require.NoError(t, err)
	}
	_, err = svc.Get(ctx, created.SessionUUID)
	require.ErrorIs(t, err, ErrNotFound)
	var count int64
	require.NoError(t, db.Model(&m.ServiceSession{}).Where("uuid = ? AND status = ?", created.SessionUUID, "deleted").Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestSessionGrantAndIdempotencyExpiry(t *testing.T) {
	svc, db, ctx, agent := fixture(t)
	_, err := svc.Create(context.Background(), agent, "")
	require.ErrorIs(t, err, ErrUnauthorized)
	bad := *reqctx.GetClaims(ctx)
	bad.Subject = "client:forged"
	_, err = svc.Create(reqctx.WithClaims(ctx, &bad), agent, "")
	require.ErrorIs(t, err, ErrUnauthorized)
	session, err := svc.Create(ctx, agent, "")
	require.NoError(t, err)
	now := time.Now()
	svc.now = func() time.Time { return now }
	_, err = svc.Append(ctx, session.SessionUUID, "key", "user", "content")
	require.NoError(t, err)
	svc.now = func() time.Time { return now.Add(IdempotencyTTL) }
	_, err = svc.Append(ctx, session.SessionUUID, "key", "user", "content")
	require.ErrorIs(t, err, ErrExpired)
	require.NoError(t, db.Model(&setting.PluginInstanceConfig{}).Where("plugin_id = ?", "plugin.test").Update("value_json", datatypes.JSON([]byte(`{"client_id":"plugin.test","allowed_capabilities":[]}`))).Error)
	_, err = svc.Get(ctx, session.SessionUUID)
	require.ErrorIs(t, err, ErrForbidden)
	_, err = NewService(nil).Get(ctx, session.SessionUUID)
	require.ErrorIs(t, err, ErrDependency)
}
