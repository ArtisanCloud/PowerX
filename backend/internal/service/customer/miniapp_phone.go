package customer

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	customerrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/customer"
	"github.com/ArtisanCloud/PowerX/pkg/corex/iam/reqctx"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// 小程序手机号凭证类型：按钮 open-type=getPhoneNumber 返回的一次性 code。
const (
	WeChatMiniAppPhoneCredentialType = "wechat_miniapp_phone_code"
	AlipayMiniAppPhoneCredentialType = "alipay_miniapp_phone_code"
	DouyinMiniAppPhoneCredentialType = "douyin_miniapp_phone_code"

	weChatStableTokenURL = "https://api.weixin.qq.com/cgi-bin/stable_token"
	weChatPhoneNumberURL = "https://api.weixin.qq.com/wxa/business/getuserphonenumber"
)

var miniAppPhoneCredentialTypes = map[string]string{
	WeChatMiniAppChannel: WeChatMiniAppPhoneCredentialType,
	AlipayMiniAppChannel: AlipayMiniAppPhoneCredentialType,
	DouyinMiniAppChannel: DouyinMiniAppPhoneCredentialType,
}

// MiniAppPhoneCredentialType 返回平台对应的手机号 code 凭证类型。
func MiniAppPhoneCredentialType(platform string) (string, bool) {
	t, ok := miniAppPhoneCredentialTypes[strings.TrimSpace(platform)]
	return t, ok
}

// CustomerPhoneVerifyCapabilityID 是插件为当前客户认证手机号所需的能力。
const CustomerPhoneVerifyCapabilityID = "com.corex.customer.phone.verify"

var (
	ErrCustomerPhoneCodeInvalid = errors.New("customer.phone_code_invalid")
	ErrCustomerPhoneConflict    = errors.New("customer.phone_conflict")
)

// PhoneInfo 是平台返回的手机号。
type PhoneInfo struct {
	PurePhone   string
	CountryCode string
}

// E164 返回 +<国家码><号码>。
func (p PhoneInfo) E164() string {
	return "+" + strings.TrimPrefix(strings.TrimSpace(p.CountryCode), "+") + strings.TrimSpace(p.PurePhone)
}

// MaskPhone 只保留前 3 位与后 4 位，例如 138****0000。
func MaskPhone(pure string) string {
	pure = strings.TrimSpace(pure)
	if len(pure) <= 7 {
		return strings.Repeat("*", len(pure))
	}
	return pure[:3] + strings.Repeat("*", len(pure)-7) + pure[len(pure)-4:]
}

// PhoneVerification 是手机号认证结果（对插件只返回脱敏号码）。
type PhoneVerification struct {
	PhoneMasked string    `json:"phone_masked"`
	VerifiedAt  time.Time `json:"verified_at"`
}

// MiniAppPhoneVerifier 用客户端应用的凭证把手机号 code 换成手机号（各平台一个实现）。
type MiniAppPhoneVerifier interface {
	VerifyPhone(ctx context.Context, app ClientApp, code string) (PhoneInfo, error)
}

// VerifyMiniAppPhone 为当前客户认证手机号：校验插件可用该应用 → 平台换出手机号 → 已验证号码不得被他人占用 →
// 写入账户 primary_phone、验证标记与主联系人手机号，并记审计。重复认证同一号码是幂等的。
func (s *CustomerAuthService) VerifyMiniAppPhone(ctx context.Context, tenantUUID, customerUUID, platform, appID, code string) (PhoneVerification, error) {
	app, pluginID, err := s.resolveClientApp(ctx, tenantUUID, platform, appID)
	if err != nil {
		return PhoneVerification{}, err
	}
	// 在消费一次性上游 code 前拒绝已停用或过期的客户。
	if _, err := s.memberships.ResolveCurrent(ctx, tenantUUID, customerUUID); err != nil {
		return PhoneVerification{}, err
	}
	verifier, ok := s.phoneVerifiers[app.Platform]
	if !ok {
		return PhoneVerification{}, ErrCustomerClientPlatformUnsupported
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return PhoneVerification{}, ErrCustomerPhoneCodeInvalid
	}
	info, err := verifier.VerifyPhone(ctx, app, code)
	if err != nil {
		return PhoneVerification{}, err
	}
	phone := info.E164()
	now := time.Now().UTC()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := customerrepo.NewAccountRepository(tx)
		if err := repo.LockPhoneVerification(ctx, phone); err != nil {
			return ErrCustomerUpstreamDependency
		}
		account, membership, err := repo.LockPhoneAccount(ctx, tenantUUID, customerUUID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCustomerMembershipNotFound
			}
			return ErrCustomerUpstreamDependency
		}
		// 上游调用期间可能发生停用或到期，必须在持锁事务中再次判断。
		if account.Status != modelcustomer.StatusActive || membership.Status != modelcustomer.StatusActive || (membership.ExpiresAt != nil && !membership.ExpiresAt.After(time.Now())) {
			return ErrCustomerMembershipInactive
		}
		others, err := repo.OtherPhoneAccounts(ctx, tenantUUID, customerUUID, phone)
		if err != nil {
			return ErrCustomerUpstreamDependency
		}
		for _, other := range others {
			if other.HasVerifiedPhone() {
				return ErrCustomerPhoneConflict
			}
		}
		meta := map[string]any{}
		if len(account.Metadata) > 0 {
			if err := json.Unmarshal(account.Metadata, &meta); err != nil {
				return ErrCustomerUpstreamDependency
			}
		}
		if meta == nil {
			meta = map[string]any{}
		}
		meta[modelcustomer.PhoneVerifiedAtKey] = now.Format(time.RFC3339)
		meta[modelcustomer.PhoneVerifiedViaKey] = app.Platform + ":" + app.AppID
		meta[modelcustomer.PhoneVerifiedHashKey] = modelcustomer.PhoneVerificationHash(phone)
		raw, _ := json.Marshal(meta)
		changes, _ := json.Marshal(map[string]any{"fields": []string{"primary_phone"}, "verified_via": app.Platform + ":" + app.AppID, "plugin_id": pluginID})
		if err := repo.SaveVerifiedPhone(ctx, account, membership, phone, datatypes.JSON(raw), &modelaudit.AuditEvent{OccurredAt: now, TenantUUID: tenantUUID, Source: "customer.auth.phone", Operation: "customer.account.phone_verified", ResourceType: "customer.account", ResourceID: customerUUID, Outcome: "SUCCESS", Severity: "INFO", CorrelationID: reqctx.GetTraceID(ctx), ChangesAfter: datatypes.JSON(changes), ChangesRedacted: true}); err != nil {
			return ErrCustomerUpstreamDependency
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return PhoneVerification{}, err
	}
	return PhoneVerification{PhoneMasked: MaskPhone(info.PurePhone), VerifiedAt: now}, nil
}

// PhoneVerified 返回当前号码是否具有匹配的认证证据；缺少号码绑定的旧标记不可信。
func (s *CustomerAuthService) PhoneVerified(ctx context.Context, customerUUID string) (bool, error) {
	account, err := customerrepo.NewAccountRepository(s.db).PhoneAccount(ctx, customerUUID)
	if err != nil {
		return false, ErrCustomerUpstreamDependency
	}
	return account.HasVerifiedPhone(), nil
}

// ---- 微信 ----

// weChatAccessTokens 缓存各小程序的 access_token。使用 stable_token：多实例各自获取不会互相作废。
type weChatAccessTokens struct {
	mu     sync.Mutex
	tokens map[string]cachedToken
}

type cachedToken struct {
	value             string
	expiresAt         time.Time
	credentialVersion int64
}

const weChatTokenRefreshMargin = 5 * time.Minute

func (c *weChatAccessTokens) get(ctx context.Context, client *http.Client, app ClientApp, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := app.CredentialScope + "|" + app.Platform + "|" + app.AppID
	if t, ok := c.tokens[key]; ok && t.credentialVersion == app.CredentialVersion && !force && time.Now().Before(t.expiresAt) {
		return t.value, nil
	}
	body, _ := json.Marshal(map[string]any{"grant_type": "client_credential", "appid": app.AppID, "secret": app.Secret, "force_refresh": force})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, weChatStableTokenURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", ErrCustomerUpstreamDependency
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&out) != nil || out.ErrCode != 0 || out.AccessToken == "" {
		return "", ErrCustomerUpstreamDependency
	}
	ttl := time.Duration(out.ExpiresIn)*time.Second - weChatTokenRefreshMargin
	if ttl <= 0 {
		ttl = time.Minute
	}
	if c.tokens == nil {
		c.tokens = map[string]cachedToken{}
	}
	c.tokens[key] = cachedToken{value: out.AccessToken, expiresAt: time.Now().Add(ttl), credentialVersion: app.CredentialVersion}
	return out.AccessToken, nil
}

// weChatPhoneNumber 是微信小程序的手机号校验器（getuserphonenumber）。
type weChatPhoneNumber struct {
	service *CustomerAuthService
	tokens  *weChatAccessTokens
}

func (v weChatPhoneNumber) VerifyPhone(ctx context.Context, app ClientApp, code string) (PhoneInfo, error) {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := v.tokens.get(ctx, v.service.client, app, attempt > 0)
		if err != nil {
			return PhoneInfo{}, err
		}
		info, retry, err := v.call(ctx, token, code)
		if !retry {
			return info, err
		}
	}
	return PhoneInfo{}, ErrCustomerUpstreamDependency
}

// call 返回 retry=true 表示 access_token 失效，需要强制刷新后重试一次。
func (v weChatPhoneNumber) call(ctx context.Context, token, code string) (PhoneInfo, bool, error) {
	body, _ := json.Marshal(map[string]string{"code": code})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, weChatPhoneNumberURL+"?"+url.Values{"access_token": {token}}.Encode(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := v.service.client.Do(req)
	if err != nil {
		return PhoneInfo{}, false, ErrCustomerUpstreamDependency
	}
	defer resp.Body.Close()
	var out struct {
		ErrCode   int    `json:"errcode"`
		ErrMsg    string `json:"errmsg"`
		PhoneInfo struct {
			PhoneNumber     string `json:"phoneNumber"`
			PurePhoneNumber string `json:"purePhoneNumber"`
			CountryCode     string `json:"countryCode"`
		} `json:"phone_info"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&out) != nil {
		return PhoneInfo{}, false, ErrCustomerUpstreamDependency
	}
	switch out.ErrCode {
	case 0:
	case 40001, 42001:
		return PhoneInfo{}, true, nil
	default:
		// code 无效、已使用或过期（如 40029）。
		return PhoneInfo{}, false, ErrCustomerPhoneCodeInvalid
	}
	pure, cc := strings.TrimSpace(out.PhoneInfo.PurePhoneNumber), strings.TrimSpace(out.PhoneInfo.CountryCode)
	if pure == "" || cc == "" {
		return PhoneInfo{}, false, ErrCustomerPhoneCodeInvalid
	}
	return PhoneInfo{PurePhone: pure, CountryCode: cc}, false, nil
}
