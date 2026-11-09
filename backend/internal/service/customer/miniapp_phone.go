package customer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	modelaudit "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/audit"
	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
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

	// 账户 metadata 中的手机号验证标记。
	metaPhoneVerifiedAt  = "phone_verified_at"
	metaPhoneVerifiedVia = "phone_verified_via"
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
		var membership modelcustomer.TenantMembership
		if err := tx.Where("tenant_uuid = ? AND customer_uuid = ?", tenantUUID, customerUUID).First(&membership).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCustomerMembershipNotFound
			}
			return ErrCustomerUpstreamDependency
		}
		if err := ensurePhoneNotTaken(tx, tenantUUID, customerUUID, phone); err != nil {
			return err
		}
		var account modelcustomer.Account
		if err := tx.Where("uuid = ?", customerUUID).First(&account).Error; err != nil {
			return ErrCustomerUpstreamDependency
		}
		meta := map[string]any{}
		if len(account.Metadata) > 0 {
			_ = json.Unmarshal(account.Metadata, &meta)
		}
		meta[metaPhoneVerifiedAt] = now.Format(time.RFC3339)
		meta[metaPhoneVerifiedVia] = app.Platform + ":" + app.AppID
		raw, _ := json.Marshal(meta)
		if err := tx.Model(&modelcustomer.Account{}).Where("uuid = ?", customerUUID).Updates(map[string]any{"primary_phone": phone, "metadata": datatypes.JSON(raw)}).Error; err != nil {
			return ErrCustomerUpstreamDependency
		}
		if strings.TrimSpace(membership.PrimaryContactUUID) != "" {
			if err := tx.Model(&modelcustomer.Contact{}).Where("uuid = ? AND tenant_uuid = ?", membership.PrimaryContactUUID, tenantUUID).Update("phone", phone).Error; err != nil {
				return ErrCustomerUpstreamDependency
			}
		}
		changes, _ := json.Marshal(map[string]any{"fields": []string{"primary_phone"}, "verified_via": app.Platform + ":" + app.AppID, "plugin_id": pluginID})
		return tx.Create(&modelaudit.AuditEvent{OccurredAt: now, TenantUUID: tenantUUID, Source: "customer.auth.phone", Operation: "customer.account.phone_verified", ResourceType: "customer.account", ResourceID: customerUUID, Outcome: "SUCCESS", Severity: "INFO", CorrelationID: reqctx.GetTraceID(ctx), ChangesAfter: datatypes.JSON(changes), ChangesRedacted: true}).Error
	})
	if err != nil {
		return PhoneVerification{}, err
	}
	return PhoneVerification{PhoneMasked: MaskPhone(info.PurePhone), VerifiedAt: now}, nil
}

// ensurePhoneNotTaken：同一租户内其他客户已验证同一号码时拒绝；未验证的号码（如导入数据）不算冲突。
func ensurePhoneNotTaken(tx *gorm.DB, tenantUUID, customerUUID, phone string) error {
	var others []modelcustomer.Account
	members := tx.Model(&modelcustomer.TenantMembership{}).Select("customer_uuid").Where("tenant_uuid = ?", tenantUUID)
	if err := tx.Where("primary_phone = ? AND uuid <> ? AND uuid IN (?)", phone, customerUUID, members).Find(&others).Error; err != nil {
		return ErrCustomerUpstreamDependency
	}
	for _, other := range others {
		if phoneVerified(other.Metadata) {
			return ErrCustomerPhoneConflict
		}
	}
	return nil
}

func phoneVerified(metadata datatypes.JSON) bool {
	if len(metadata) == 0 {
		return false
	}
	meta := map[string]any{}
	if err := json.Unmarshal(metadata, &meta); err != nil {
		return false
	}
	v, _ := meta[metaPhoneVerifiedAt].(string)
	return strings.TrimSpace(v) != ""
}

// PhoneVerified 返回客户是否已完成手机号认证（登录响应据此决定是否引导认证）。
func (s *CustomerAuthService) PhoneVerified(ctx context.Context, customerUUID string) (bool, error) {
	var account modelcustomer.Account
	if err := s.db.WithContext(ctx).Select("metadata").Where("uuid = ?", customerUUID).First(&account).Error; err != nil {
		return false, ErrCustomerUpstreamDependency
	}
	return phoneVerified(account.Metadata), nil
}

// ---- 微信 ----

// weChatAccessTokens 缓存各小程序的 access_token。使用 stable_token：多实例各自获取不会互相作废。
type weChatAccessTokens struct {
	mu     sync.Mutex
	tokens map[string]cachedToken
}

type cachedToken struct {
	value     string
	expiresAt time.Time
}

const weChatTokenRefreshMargin = 5 * time.Minute

func (c *weChatAccessTokens) get(ctx context.Context, client *http.Client, app ClientApp, force bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := app.Platform + "|" + app.AppID
	if t, ok := c.tokens[key]; ok && !force && time.Now().Before(t.expiresAt) {
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
	c.tokens[key] = cachedToken{value: out.AccessToken, expiresAt: time.Now().Add(ttl)}
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
