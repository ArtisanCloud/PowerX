package customer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	audit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	model "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	setting "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/setting"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/ArtisanCloud/PowerX/pkg/corex/tenantkeys"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func clientAppTestService(t *testing.T) (*ClientApplicationService, *gorm.DB, string, string) {
	t.Helper()
	db := newCustomerServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ClientApplication{}, &model.ClientAppBinding{}, &audit.AuditEvent{}, &setting.PluginInstanceConfig{}))
	require.NoError(t, db.Exec(`CREATE TABLE main.iam_tenant_key_pairs (id INTEGER PRIMARY KEY AUTOINCREMENT, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME, env TEXT, tenant_uuid TEXT, k_id TEXT UNIQUE, alg TEXT, public_pem TEXT, enc_private TEXT, active BOOLEAN)`).Error)
	wrapper, err := tenantkeys.NewDirectWrapper("client-app-test", "", []byte("01234567890123456789012345678901"))
	require.NoError(t, err)
	a, b := uuid.NewString(), uuid.NewString()
	for _, tenant := range []string{a, b} {
		require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: tenant, PluginID: "com.example.shop", Key: "auth.credentials", Enabled: true, Status: "enabled", ValueJSON: datatypes.JSON([]byte(`{}`))}).Error)
	}
	return NewClientApplicationService(db, tenantkeys.NewTenantKeyServiceWithWrapper(db, wrapper)), db, a, b
}
func clientAppInput(secret string) ClientApplicationInput {
	var credential *string
	if secret != "" {
		credential = &secret
	}
	return ClientApplicationInput{Name: "Store mini program", Kind: ClientAppMiniProgram, Status: "active", PluginIDs: []string{"com.example.shop"}, Connections: []ClientAppConnectionInput{{Provider: "wechat", AppID: "wx-test-application", AppSecret: credential}}}
}
func clientAppUpdate(view ClientApplicationView) ClientApplicationInput {
	in := clientAppInput("")
	in.ExpectedVersion = view.Version
	in.Connections[0].UUID = view.Connections[0].UUID
	return in
}

func TestClientApplicationTenantPersistenceAndCredentials(t *testing.T) {
	s, db, a, b := clientAppTestService(t)
	ctxA, ctxB := importAdminContext(a), importAdminContext(b)
	first, err := s.Save(ctxA, "", clientAppInput("tenant-a-secret"))
	require.NoError(t, err)
	second, err := s.Save(ctxB, "", clientAppInput("tenant-b-secret"))
	require.NoError(t, err)
	require.NotEqual(t, first.UUID, second.UUID, "same provider AppID can be independently registered by two tenants")
	wire, err := json.Marshal(first)
	require.NoError(t, err)
	require.NotContains(t, string(wire), "tenant-a-secret")
	require.NotContains(t, string(wire), "sealed_secret")
	var binding model.ClientAppBinding
	require.NoError(t, db.Where("tenant_uuid = ?", a).First(&binding).Error)
	sealed, _ := json.Marshal(binding.SealedSecret)
	require.NotContains(t, string(sealed), "tenant-a-secret")
	var events []audit.AuditEvent
	require.NoError(t, db.Find(&events).Error)
	eventsJSON, _ := json.Marshal(events)
	require.NotContains(t, string(eventsJSON), "tenant-a-secret")
	var wrongScope struct {
		AppSecret string `json:"app_secret"`
	}
	require.Error(t, s.keys.UnsealSensitive(ctxB, "default", b, binding.SealedSecret, &wrongScope), "another tenant key cannot decrypt this credential")
	_, err = s.Get(ctxB, first.UUID)
	require.Equal(t, 404, dto.StatusCode(err))
	_, err = s.Save(ctxB, first.UUID, clientAppUpdate(first))
	require.Equal(t, 404, dto.StatusCode(err))
	rows, total, err := s.List(ctxA, "", "wechat", "", 1, 20)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, first.UUID, rows[0].UUID)
	restarted := NewClientApplicationService(db, s.keys)
	runtime, ok, err := restarted.Lookup(context.Background(), a, "com.example.shop", WeChatMiniAppChannel, "wx-test-application")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "tenant-a-secret", runtime.Secret)
	require.Equal(t, a, runtime.CredentialScope)
	_, _, err = s.Lookup(context.Background(), a, "com.example.other", WeChatMiniAppChannel, "wx-test-application")
	require.ErrorIs(t, err, ErrCustomerClientAppForbidden)
	update := clientAppUpdate(first)
	update.Status = "disabled"
	disabled, err := s.Save(ctxA, first.UUID, update)
	require.NoError(t, err)
	require.EqualValues(t, 2, disabled.Version)
	s.SetLegacyApplications([]ClientApp{{Platform: WeChatMiniAppChannel, AppID: "wx-test-application", Secret: "old-config-secret", PluginIDs: []string{"com.example.shop"}}})
	_, _, err = s.Lookup(context.Background(), a, "com.example.shop", WeChatMiniAppChannel, "wx-test-application")
	require.ErrorIs(t, err, ErrCustomerClientAppForbidden, "disabled app must never fall back to deployment config")
	_, err = s.Save(ctxA, first.UUID, update)
	require.Equal(t, "CLIENT_APP_VERSION_CONFLICT", dto.CodeOf(err))
	rotate := clientAppUpdate(disabled)
	secret := "rotated-secret"
	rotate.Connections[0].AppSecret = &secret
	updated, err := s.Save(ctxA, first.UUID, rotate)
	require.NoError(t, err)
	require.EqualValues(t, 3, updated.Version)
	runtime, ok, err = s.Lookup(context.Background(), a, "com.example.shop", WeChatMiniAppChannel, "wx-test-application")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, secret, runtime.Secret)
	require.EqualValues(t, 3, runtime.CredentialVersion)
	require.NoError(t, db.Model(&setting.PluginInstanceConfig{}).Where("tenant_uuid = ?", a).Updates(map[string]any{"enabled": false, "status": "disabled"}).Error)
	_, _, err = s.Lookup(context.Background(), a, "com.example.shop", WeChatMiniAppChannel, "wx-test-application")
	require.ErrorIs(t, err, ErrCustomerClientAppForbidden)
}

func TestClientApplicationAdminAndPluginBoundaries(t *testing.T) {
	s, db, a, b := clientAppTestService(t)
	ctx := importAdminContext(a)
	require.NoError(t, db.Create(&setting.PluginInstanceConfig{TenantUUID: b, PluginID: "com.example.foreign", Key: "auth.credentials", Enabled: true, Status: "enabled"}).Error)
	in := clientAppInput("test-secret")
	in.PluginIDs = []string{"com.example.foreign"}
	_, err := s.Save(ctx, "", in)
	require.Equal(t, "CLIENT_APP_PLUGIN_FORBIDDEN", dto.CodeOf(err))
	in = clientAppInput("test-secret")
	in.Connections[0].Provider = "alipay"
	_, err = s.Save(ctx, "", in)
	require.Equal(t, "CLIENT_APP_PROVIDER_UNSUPPORTED", dto.CodeOf(err))
	in = clientAppInput(strings.Repeat("x", 65))
	_, err = s.Save(ctx, "", in)
	require.Equal(t, "CLIENT_APP_SECRET_INVALID", dto.CodeOf(err))
	in = clientAppInput("")
	_, err = s.Save(ctx, "", in)
	require.Equal(t, "CLIENT_APP_SECRET_REQUIRED", dto.CodeOf(err))
	for _, claims := range []*reqctx.CoreXClaims{{Platforms: []string{"api_key"}}, {}} {
		if len(claims.Platforms) == 0 {
			claims.Issuer = "powerx-sts"
		}
		bad := reqctx.WithClaims(ctx, claims)
		_, err = s.Save(bad, "", clientAppInput("test-secret"))
		require.Equal(t, 403, dto.StatusCode(err))
		_, err = s.Catalog(bad)
		require.Equal(t, 403, dto.StatusCode(err))
	}
	s.SetLegacyApplications([]ClientApp{{Name: "Legacy", Platform: WeChatMiniAppChannel, AppID: "wx-legacy", Secret: "legacy-secret", TenantUUIDs: []string{a}, PluginIDs: []string{"com.example.shop", "com.example.foreign"}}})
	cat, err := s.Catalog(importAdminContext(b))
	require.NoError(t, err)
	require.Empty(t, cat.LegacyApplications)
	_, err = s.ImportLegacy(importAdminContext(b), WeChatMiniAppChannel, "wx-legacy")
	require.Equal(t, 404, dto.StatusCode(err))
	migrated, err := s.ImportLegacy(ctx, WeChatMiniAppChannel, "wx-legacy")
	require.NoError(t, err)
	require.Equal(t, []string{"com.example.shop"}, migrated.PluginIDs)
	_, err = s.ImportLegacy(ctx, WeChatMiniAppChannel, "wx-legacy")
	require.Equal(t, "CLIENT_APP_DUPLICATE", dto.CodeOf(err))
	require.NoError(t, db.Migrator().DropTable(&model.ClientApplication{}))
	_, _, err = s.Lookup(ctx, a, "com.example.shop", WeChatMiniAppChannel, "wx-legacy")
	require.ErrorIs(t, err, ErrCustomerUpstreamDependency)
}

func TestClientApplicationTokenCacheSeparatesTenantAndRotation(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: customerRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(fmt.Sprintf(`{"access_token":"token-%d","expires_in":7200}`, calls)))}, nil
	})}
	cache := &weChatAccessTokens{}
	a := ClientApp{Platform: WeChatMiniAppChannel, AppID: "same-app", Secret: "a", CredentialScope: "tenant-a", CredentialVersion: 1}
	token, err := cache.get(context.Background(), client, a, false)
	require.NoError(t, err)
	require.Equal(t, "token-1", token)
	_, err = cache.get(context.Background(), client, a, false)
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	b := a
	b.CredentialScope = "tenant-b"
	b.Secret = "b"
	token, err = cache.get(context.Background(), client, b, false)
	require.NoError(t, err)
	require.Equal(t, "token-2", token)
	a.CredentialVersion++
	a.Secret = "rotated"
	token, err = cache.get(context.Background(), client, a, false)
	require.NoError(t, err)
	require.Equal(t, "token-3", token)
}
