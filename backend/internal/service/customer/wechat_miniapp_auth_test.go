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
	weChatTestTenant = "11111111-1111-1111-1111-111111111111"
	weChatTestPlugin = "com.powerx.plugins.scora"
	weChatTestAppID  = "wx-test-app"
)

func weChatTestContext(pluginID string) context.Context {
	return reqctx.WithClaims(reqctx.WithTenantUUID(context.Background(), weChatTestTenant), &reqctx.CoreXClaims{
		TenantUUID: weChatTestTenant,
		PluginID:   pluginID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "powerx-sts",
			Audience: jwt.ClaimStrings{"powerx:api"},
		},
	})
}

// weChatTestService 用假的 code2session：code "code-<openid>" 换出对应 openid，"bad" 返回 errcode。
func weChatTestService(t *testing.T, unionID string) (*CustomerAuthService, *int) {
	t.Helper()
	db := newCustomerServiceTestDB(t)
	tokens := NewCustomerTokenService(db, []byte("customer-test-secret"), "powerx-auth", time.Hour)
	calls := 0
	service := NewCustomerAuthService(db, tokens).WithWeChatMiniApp(WeChatMiniAppConfig{AppID: weChatTestAppID, AppSecret: "core-owned-secret"})
	service.client = &http.Client{Transport: customerRoundTripper(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "api.weixin.qq.com", req.URL.Host)
		require.Equal(t, "/sns/jscode2session", req.URL.Path)
		q := req.URL.Query()
		require.Equal(t, weChatTestAppID, q.Get("appid"))
		require.Equal(t, "core-owned-secret", q.Get("secret"))
		require.Equal(t, "authorization_code", q.Get("grant_type"))
		body := `{"errcode":40029,"errmsg":"invalid code"}`
		if openID, ok := strings.CutPrefix(q.Get("js_code"), "code-"); ok {
			payload, _ := json.Marshal(map[string]string{"openid": openID, "session_key": "sk", "unionid": unionID})
			body = string(payload)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/plain"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	return service, &calls
}

func TestWeChatMiniAppRegisterIsIdempotentAndTracksPluginSource(t *testing.T) {
	service, _ := weChatTestService(t, "union-1")
	ctx := weChatTestContext(weChatTestPlugin)

	pair, membership, err := service.RegisterWeChatMiniApp(ctx, weChatTestTenant, "code-openid-0001")
	require.NoError(t, err)
	require.NotEmpty(t, pair.AccessToken)
	require.NotEmpty(t, pair.RefreshToken)
	require.Equal(t, weChatTestTenant, membership.TenantUUID)
	require.NotEmpty(t, membership.PrimaryContactUUID)

	provider, err := customerrepo.ExternalIdentityProviderKey(weChatTestPlugin)
	require.NoError(t, err)
	var identity modelcustomer.AuthIdentity
	require.NoError(t, service.db.Where("provider = ? AND provider_subject = ?", provider, "wechat:miniapp:wx-test-app:openid-0001").First(&identity).Error)
	require.Equal(t, membership.CustomerUUID, identity.CustomerUUID)
	require.NotNil(t, identity.VerifiedAt)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(identity.Metadata, &meta))
	require.Equal(t, "union-1", meta["wechat_unionid"])
	require.Equal(t, weChatTestPlugin, meta["provider_plugin_id"])

	var tm modelcustomer.TenantMembership
	require.NoError(t, service.db.Where("tenant_uuid = ? AND customer_uuid = ?", weChatTestTenant, membership.CustomerUUID).First(&tm).Error)
	require.Equal(t, "plugin_identity", tm.Source)
	var account modelcustomer.Account
	require.NoError(t, service.db.Where("uuid = ?", membership.CustomerUUID).First(&account).Error)
	require.Equal(t, "微信用户0001", account.DisplayName)

	// 同一 openid 再次登录/注册返回同一客户，不重复建档。
	_, again, err := service.RegisterWeChatMiniApp(ctx, weChatTestTenant, "code-openid-0001")
	require.NoError(t, err)
	require.Equal(t, membership.CustomerUUID, again.CustomerUUID)
	_, loggedIn, err := service.LoginWeChatMiniApp(ctx, weChatTestTenant, "code-openid-0001")
	require.NoError(t, err)
	require.Equal(t, membership.CustomerUUID, loggedIn.CustomerUUID)
	var count int64
	require.NoError(t, service.db.Model(&modelcustomer.AuthIdentity{}).Where("provider = ?", provider).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestWeChatMiniAppLoginDoesNotCreateCustomer(t *testing.T) {
	service, _ := weChatTestService(t, "")
	_, _, err := service.LoginWeChatMiniApp(weChatTestContext(weChatTestPlugin), weChatTestTenant, "code-openid-new")
	require.ErrorIs(t, err, ErrCustomerIdentityNotFound)
}

func TestWeChatMiniAppRejectsInvalidCodeAndMissingConfig(t *testing.T) {
	service, calls := weChatTestService(t, "")
	ctx := weChatTestContext(weChatTestPlugin)

	_, _, err := service.RegisterWeChatMiniApp(ctx, weChatTestTenant, "bad")
	require.ErrorIs(t, err, ErrCustomerCredentialInvalid)

	_, _, err = service.RegisterWeChatMiniApp(context.Background(), weChatTestTenant, "code-openid-x")
	require.ErrorIs(t, err, ErrCustomerUnauthorized, "必须是插件 STS 凭证调用")

	before := *calls
	service.WithWeChatMiniApp(WeChatMiniAppConfig{})
	_, _, err = service.RegisterWeChatMiniApp(ctx, weChatTestTenant, "code-openid-x")
	require.ErrorIs(t, err, ErrCustomerUpstreamDependency)
	require.Equal(t, before, *calls, "未配置时不应调用微信")
}

func TestWeChatMiniAppIdentitiesAreScopedPerPlugin(t *testing.T) {
	service, _ := weChatTestService(t, "")
	_, a, err := service.RegisterWeChatMiniApp(weChatTestContext(weChatTestPlugin), weChatTestTenant, "code-openid-same")
	require.NoError(t, err)
	_, b, err := service.RegisterWeChatMiniApp(weChatTestContext("com.powerx.plugins.other"), weChatTestTenant, "code-openid-same")
	require.NoError(t, err)
	require.NotEqual(t, a.CustomerUUID, b.CustomerUUID, "不同插件的同一 openid 不共享客户")
}
