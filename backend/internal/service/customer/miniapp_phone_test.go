package customer

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	coremodel "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model"
	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"github.com/stretchr/testify/require"
)

// phoneTestServer 模拟微信：jscode2session（code "<appid>/<openid>"）、stable_token、getuserphonenumber（code "phone-<号码>"）。
type phoneTestServer struct {
	tokenCalls   int
	phoneCalls   int
	expireNext   bool // 下一次换手机号返回 40001（token 失效）
	currentToken string
}

func miniAppPhoneTestService(t *testing.T) (*CustomerAuthService, *phoneTestServer) {
	t.Helper()
	service, _ := miniAppTestService(t, "")
	require.NoError(t, service.db.Exec(`CREATE TABLE IF NOT EXISTS main.`+coremodel.TableAuditEvent+` (
		id INTEGER PRIMARY KEY AUTOINCREMENT, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME, occurred_at DATETIME NOT NULL,
		tenant_uuid TEXT NOT NULL, correlation_id TEXT, source TEXT NOT NULL, operation TEXT NOT NULL, resource_type TEXT NOT NULL,
		resource_id TEXT NOT NULL, resource_name TEXT, outcome TEXT NOT NULL, severity TEXT NOT NULL, actor_user_id INTEGER,
		actor_user_name TEXT, actor_display TEXT, actor_role_ids TEXT, client_ip TEXT, client_ua TEXT, changes_before TEXT,
		changes_after TEXT, changes_diff TEXT, changes_redacted BOOLEAN DEFAULT false, meta TEXT, hash TEXT, prev_hash TEXT)`).Error)
	srv := &phoneTestServer{}
	service.client = &http.Client{Transport: customerRoundTripper(func(req *http.Request) (*http.Response, error) {
		respond := func(v any) (*http.Response, error) {
			raw, _ := json.Marshal(v)
			return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
		}
		switch req.URL.Path {
		case "/sns/jscode2session":
			q := req.URL.Query()
			if appID, openID, ok := strings.Cut(q.Get("js_code"), "/"); ok && appID == q.Get("appid") {
				return respond(map[string]string{"openid": openID, "session_key": "sk"})
			}
			return respond(map[string]any{"errcode": 40029, "errmsg": "invalid code"})
		case "/cgi-bin/stable_token":
			var body map[string]any
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			require.Equal(t, miniAppScoraAppID, body["appid"])
			require.Equal(t, miniAppScoraSecret, body["secret"], "必须使用该应用自己的密钥")
			srv.tokenCalls++
			srv.currentToken = "token-" + string(rune('0'+srv.tokenCalls))
			return respond(map[string]any{"access_token": srv.currentToken, "expires_in": 7200})
		case "/wxa/business/getuserphonenumber":
			srv.phoneCalls++
			if srv.expireNext {
				srv.expireNext = false
				return respond(map[string]any{"errcode": 40001, "errmsg": "invalid credential"})
			}
			require.Equal(t, srv.currentToken, req.URL.Query().Get("access_token"))
			var body map[string]string
			require.NoError(t, json.NewDecoder(req.Body).Decode(&body))
			if pure, ok := strings.CutPrefix(body["code"], "phone-"); ok {
				return respond(map[string]any{"errcode": 0, "phone_info": map[string]string{"phoneNumber": "+86" + pure, "purePhoneNumber": pure, "countryCode": "86"}})
			}
			return respond(map[string]any{"errcode": 40029, "errmsg": "invalid code"})
		}
		t.Fatalf("unexpected request %s", req.URL)
		return nil, nil
	})}
	return service, srv
}

func TestMiniAppPhoneVerifyBindsVerifiedPhone(t *testing.T) {
	service, srv := miniAppPhoneTestService(t)
	ctx := miniAppTestContext(miniAppTestPlugin, miniAppTestTenant)
	_, m, err := service.RegisterMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/openid-phone")
	require.NoError(t, err)
	verified, err := service.PhoneVerified(ctx, m.CustomerUUID)
	require.NoError(t, err)
	require.False(t, verified, "新客户尚未认证手机号")

	out, err := service.VerifyMiniAppPhone(ctx, miniAppTestTenant, m.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, "phone-13800001234")
	require.NoError(t, err)
	require.Equal(t, "138****1234", out.PhoneMasked)
	require.WithinDuration(t, time.Now(), out.VerifiedAt, time.Minute)

	var account modelcustomer.Account
	require.NoError(t, service.db.Where("uuid = ?", m.CustomerUUID).First(&account).Error)
	require.Equal(t, "+8613800001234", account.PrimaryPhone)
	var meta map[string]any
	require.NoError(t, json.Unmarshal(account.Metadata, &meta))
	require.NotEmpty(t, meta["phone_verified_at"])
	require.Equal(t, "wechat_miniapp:"+miniAppScoraAppID, meta["phone_verified_via"])
	verified, err = service.PhoneVerified(ctx, m.CustomerUUID)
	require.NoError(t, err)
	require.True(t, verified)
	var contact modelcustomer.Contact
	require.NoError(t, service.db.Where("uuid = ?", m.PrimaryContactUUID).First(&contact).Error)
	require.Equal(t, "+8613800001234", contact.Phone, "主联系人手机号同步")
	var audits int64
	require.NoError(t, service.db.Model(&modelaudit.AuditEvent{}).Where("operation = ? AND resource_id = ?", "customer.account.phone_verified", m.CustomerUUID).Count(&audits).Error)
	require.EqualValues(t, 1, audits)

	// 同一客户重复认证同一号码是幂等的；access_token 被缓存，只获取一次。
	_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, m.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, "phone-13800001234")
	require.NoError(t, err)
	require.Equal(t, 1, srv.tokenCalls)

	// token 失效：强制刷新一次后重试成功。
	srv.expireNext = true
	_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, m.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, "phone-13800001234")
	require.NoError(t, err)
	require.Equal(t, 2, srv.tokenCalls)
}

func TestMiniAppPhoneVerifyRejectsConflictsAndInvalidInput(t *testing.T) {
	service, srv := miniAppPhoneTestService(t)
	ctx := miniAppTestContext(miniAppTestPlugin, miniAppTestTenant)
	_, a, err := service.RegisterMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/openid-a")
	require.NoError(t, err)
	_, b, err := service.RegisterMiniApp(ctx, miniAppTestTenant, WeChatMiniAppChannel, miniAppScoraAppID, "wx-scora/openid-b")
	require.NoError(t, err)

	_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, a.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, "phone-13900000000")
	require.NoError(t, err)
	_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, b.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, "phone-13900000000")
	require.ErrorIs(t, err, ErrCustomerPhoneConflict, "已被他人验证的号码不能再绑定")

	// 未验证的同号码（例如导入数据）不算冲突。
	require.NoError(t, service.db.Model(&modelcustomer.Account{}).Where("uuid = ?", a.CustomerUUID).Update("metadata", "{}").Error)
	_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, b.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, "phone-13900000000")
	require.NoError(t, err)

	_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, b.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, "bad-code")
	require.ErrorIs(t, err, ErrCustomerPhoneCodeInvalid)
	_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, b.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, " ")
	require.ErrorIs(t, err, ErrCustomerPhoneCodeInvalid)

	before := srv.phoneCalls
	_, err = service.VerifyMiniAppPhone(miniAppTestContext(miniAppOtherPlugin, miniAppOtherTenant), miniAppOtherTenant, b.CustomerUUID, WeChatMiniAppChannel, miniAppScoraAppID, "phone-1")
	require.ErrorIs(t, err, ErrCustomerClientAppForbidden)
	_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, b.CustomerUUID, WeChatMiniAppChannel, "wx-unknown", "phone-1")
	require.ErrorIs(t, err, ErrCustomerClientAppNotFound)
	_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, b.CustomerUUID, WeChatMiniAppChannel, miniAppUnconfiguredApp, "phone-1")
	require.ErrorIs(t, err, ErrCustomerClientAppNotConfigured)
	_, err = service.VerifyMiniAppPhone(ctx, miniAppTestTenant, b.CustomerUUID, AlipayMiniAppChannel, "2021000000000000", "phone-1")
	require.ErrorIs(t, err, ErrCustomerClientPlatformUnsupported)
	require.Equal(t, before, srv.phoneCalls, "被拒的请求不应调用微信")
}

func TestPhoneHelpers(t *testing.T) {
	require.Equal(t, "138****1234", MaskPhone("13800001234"))
	require.Equal(t, "+8613800001234", PhoneInfo{PurePhone: "13800001234", CountryCode: "86"}.E164())
	typ, ok := MiniAppPhoneCredentialType(WeChatMiniAppChannel)
	require.True(t, ok)
	require.Equal(t, WeChatMiniAppPhoneCredentialType, typ)
}
