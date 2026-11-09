package customer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

const (
	miniAppTestTenant      = "11111111-1111-1111-1111-111111111111"
	miniAppOtherTenant     = "22222222-2222-2222-2222-222222222222"
	miniAppTestPlugin      = "com.powerx.plugins.scora"
	miniAppOtherPlugin     = "com.powerx.plugins.other"
	miniAppScoraAppID      = "wx-scora"
	miniAppOtherAppID      = "wx-other"
	miniAppScoraSecret     = "scora-secret"
	miniAppOtherSecret     = "other-secret"
	miniAppUnconfiguredApp = "wx-pending"
)

func miniAppTestContext(pluginID, tenantUUID string) context.Context {
	return reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), tenantUUID), &reqctx.CoreXClaims{
		TenantUUID: tenantUUID,
		PluginID:   pluginID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "powerx-sts",
			Audience: jwt.ClaimStrings{"powerx:api"},
		},
	})
}

// miniAppTestService 注册两个小程序（各绑定一个插件），另有一个未填密钥的应用与一个支付宝应用。
// 假的 code2session：code 形如 "<appid>/<openid>"，只有用同一 AppID 及其密钥校验时才有效（模拟 code 归属某个小程序）。
func miniAppTestService(t *testing.T, unionID string) (*CustomerAuthService, *[]string) {
	t.Helper()
	db := newCustomerServiceTestDB(t)
	tokens := NewCustomerTokenService(db, []byte("customer-test-secret"), "powerx-auth", time.Hour)
	registry := NewStaticClientAppRegistry([]ClientApp{
		{Platform: WeChatMiniAppChannel, AppID: miniAppScoraAppID, Secret: miniAppScoraSecret, PluginIDs: []string{miniAppTestPlugin}},
		{Platform: WeChatMiniAppChannel, AppID: miniAppOtherAppID, Secret: miniAppOtherSecret, PluginIDs: []string{miniAppOtherPlugin}, TenantUUIDs: []string{miniAppOtherTenant}},
		{Platform: WeChatMiniAppChannel, AppID: miniAppUnconfiguredApp, PluginIDs: []string{miniAppTestPlugin}},
		{Platform: AlipayMiniAppChannel, AppID: "2021000000000000", Secret: "ali-secret", PluginIDs: []string{miniAppTestPlugin}},
	})
	secrets := map[string]string{miniAppScoraAppID: miniAppScoraSecret, miniAppOtherAppID: miniAppOtherSecret}
	var calls []string
	service := NewCustomerAuthService(db, tokens).WithClientApps(registry)
	service.client = &http.Client{Transport: customerRoundTripper(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "api.weixin.qq.com", req.URL.Host)
		require.Equal(t, "/sns/jscode2session", req.URL.Path)
		q := req.URL.Query()
		calls = append(calls, q.Get("appid"))
		require.Equal(t, secrets[q.Get("appid")], q.Get("secret"), "必须使用该应用自己的密钥")
		require.Equal(t, "authorization_code", q.Get("grant_type"))
		body := `{"errcode":40029,"errmsg":"invalid code"}`
		if appID, openID, ok := strings.Cut(q.Get("js_code"), "/"); ok && appID == q.Get("appid") {
			payload, _ := json.Marshal(map[string]string{"openid": openID, "session_key": "sk", "unionid": unionID})
			body = string(payload)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/plain"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	return service, &calls
}

func TestMiniAppRegisterIsIdempotentAndRecordsClientApp(t *testing.T) {
	service, _ := miniAppTestService(t, "union-1")
	ctx := miniAppTestContext(miniAppTestPlugin, miniAppTestTenant)

	pair, membership, err := service.RegisterMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/openid-0001")
	require.NoError(t, err)
	require.NotEmpty(t, pair.AccessToken)
	require.NotEmpty(t, pair.RefreshToken)
	require.Equal(t, miniAppTestTenant, membership.TenantUUID)

	provider, err := customerrepo.ExternalIdentityProviderKey(miniAppTestPlugin)
	require.NoError(t, err)
	var identity modelcustomer.AuthIdentity
	require.NoError(t, service.db.Where("provider = ? AND provider_subject = ?", provider, "wechat:miniapp:wx-scora:openid-0001").First(&identity).Error, "微信身份主键格式保持不变")
	require.Equal(t, membership.CustomerUUID, identity.CustomerUUID)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(identity.Metadata, &meta))
	require.Equal(t, "union-1", meta["wechat_unionid"])
	require.Equal(t, WeChatMiniAppChannel, meta["client_platform"])
	require.Equal(t, miniAppScoraAppID, meta["client_app_id"])
	require.Equal(t, miniAppTestPlugin, meta["provider_plugin_id"])

	var tm modelcustomer.TenantMembership
	require.NoError(t, service.db.Where("tenant_uuid = ? AND customer_uuid = ?", miniAppTestTenant, membership.CustomerUUID).First(&tm).Error)
	require.Equal(t, "plugin_identity", tm.Source)
	var account modelcustomer.Account
	require.NoError(t, service.db.Where("uuid = ?", membership.CustomerUUID).First(&account).Error)
	require.Equal(t, "微信用户0001", account.DisplayName)

	// 同一 openid 再次注册 / 登录返回同一客户，不重复建档。
	_, again, err := service.RegisterMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/openid-0001")
	require.NoError(t, err)
	require.Equal(t, membership.CustomerUUID, again.CustomerUUID)
	_, loggedIn, err := service.LoginMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/openid-0001")
	require.NoError(t, err)
	require.Equal(t, membership.CustomerUUID, loggedIn.CustomerUUID)
	var count int64
	require.NoError(t, service.db.Model(&modelcustomer.AuthIdentity{}).Where("provider = ?", provider).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestMiniAppLoginDoesNotCreateCustomer(t *testing.T) {
	service, _ := miniAppTestService(t, "")
	_, _, err := service.LoginMiniApp(miniAppTestContext(miniAppTestPlugin, miniAppTestTenant), miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/openid-new")
	require.ErrorIs(t, err, ErrCustomerIdentityNotFound)
}

// 两个插件各用自己的小程序：各自用自己的 AppID / 密钥校验，互不可用。
func TestMiniAppEachPluginUsesItsOwnApp(t *testing.T) {
	service, calls := miniAppTestService(t, "")
	scora := miniAppTestContext(miniAppTestPlugin, miniAppTestTenant)
	other := miniAppTestContext(miniAppOtherPlugin, miniAppOtherTenant)

	_, a, err := service.RegisterMiniApp(scora, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/openid-same")
	require.NoError(t, err)
	_, b, err := service.RegisterMiniApp(other, miniAppOtherTenant, WeChatMiniAppChannel, miniAppOtherAppID, "wx-other/openid-same")
	require.NoError(t, err)
	require.NotEqual(t, a.CustomerUUID, b.CustomerUUID)
	require.Equal(t, []string{miniAppScoraAppID, miniAppOtherAppID}, *calls)

	// 插件不能使用未授权给它的应用；应用限定租户时其他租户也不能用。
	before := len(*calls)
	_, _, err = service.RegisterMiniApp(scora, miniAppTestTenant, WeChatMiniAppChannel, miniAppOtherAppID, "wx-other/openid-x")
	require.ErrorIs(t, err, ErrCustomerClientAppForbidden)
	_, _, err = service.RegisterMiniApp(miniAppTestContext(miniAppOtherPlugin, miniAppTestTenant), miniAppTestTenant, WeChatMiniAppChannel, miniAppOtherAppID, "wx-other/openid-x")
	require.ErrorIs(t, err, ErrCustomerClientAppForbidden)
	require.Equal(t, before, len(*calls), "授权失败时不应调用微信")

	// code 属于别的小程序：用本应用凭证校验失败。
	_, _, err = service.RegisterMiniApp(scora, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-other/openid-y")
	require.ErrorIs(t, err, ErrCustomerCredentialInvalid)
}

func TestMiniAppRejectsUnknownUnconfiguredAndUnsupported(t *testing.T) {
	service, calls := miniAppTestService(t, "")
	ctx := miniAppTestContext(miniAppTestPlugin, miniAppTestTenant)

	_, _, err := service.RegisterMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, "wx-unknown", "wx-unknown/openid")
	require.ErrorIs(t, err, ErrCustomerClientAppNotFound)
	_, _, err = service.RegisterMiniApp(ctx, miniAppTestTenant, DouyinMiniAppChannel, miniAppScoraAppID, "x")
	require.ErrorIs(t, err, ErrCustomerClientAppNotFound, "平台不同即为不同应用")
	_, _, err = service.RegisterMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, miniAppUnconfiguredApp, "wx-pending/openid")
	require.ErrorIs(t, err, ErrCustomerClientAppNotConfigured)
	_, _, err = service.RegisterMiniApp(ctx, miniAppTestTenant, AlipayMiniAppChannel, "2021000000000000", "auth-code")
	require.ErrorIs(t, err, ErrCustomerClientPlatformUnsupported)
	_, _, err = service.RegisterMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, " ")
	require.ErrorIs(t, err, ErrCustomerCredentialInvalid)
	_, _, err = service.RegisterMiniApp(context.Background(), miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/openid")
	require.ErrorIs(t, err, ErrCustomerUnauthorized, "必须是插件 STS 凭证调用")
	require.Empty(t, *calls, "以上情况都不应调用微信")

	empty := NewCustomerAuthService(service.db, service.tokens)
	_, _, err = empty.RegisterMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/openid")
	require.ErrorIs(t, err, ErrCustomerClientAppNotFound, "未启用注册表时没有任何可用应用")
}

func TestMiniAppIdentitySubjects(t *testing.T) {
	require.Equal(t, "wechat:miniapp:wx1:o1", WeChatMiniAppExternalIdentitySubject("wx1", "o1"))
	require.Equal(t, "alipay:miniapp:2021:u1", MiniAppExternalIdentitySubject(AlipayMiniAppChannel, "2021", "u1"))
	require.Equal(t, "douyin:miniapp:tt1:o1", MiniAppExternalIdentitySubject(DouyinMiniAppChannel, "tt1", "o1"))
	typ, ok := MiniAppCredentialType(DouyinMiniAppChannel)
	require.True(t, ok)
	require.Equal(t, DouyinMiniAppCredentialType, typ)
	_, ok = MiniAppCredentialType(ShopifyStorefrontChannel)
	require.False(t, ok)
}
