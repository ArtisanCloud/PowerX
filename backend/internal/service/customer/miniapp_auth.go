package customer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	modelcustomer "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/customer"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const weChatCode2SessionURL = "https://api.weixin.qq.com/sns/jscode2session"

// MiniAppUser 是平台校验 code 后得到的用户标识。
type MiniAppUser struct {
	OpenID  string
	UnionID string
}

// MiniAppCodeVerifier 用客户端应用的凭证校验一次性 code（各平台一个实现）。
// 插件只转发 code 与 app_id，无法自称某个 openid。
type MiniAppCodeVerifier interface {
	Verify(ctx context.Context, app ClientApp, code string) (MiniAppUser, error)
}

// WithClientApps 启用小程序委派登录：注册表决定可用的应用，按平台注册校验器。
// 目前只有微信校验器；已登记但没有校验器的平台登录时返回 ErrCustomerClientPlatformUnsupported。
func (s *CustomerAuthService) WithClientApps(registry ClientAppRegistry) *CustomerAuthService {
	if s != nil {
		s.clientApps = registry
		s.verifiers = map[string]MiniAppCodeVerifier{WeChatMiniAppChannel: weChatCode2Session{service: s}}
		s.phoneVerifiers = map[string]MiniAppPhoneVerifier{WeChatMiniAppChannel: weChatPhoneNumber{service: s, tokens: &weChatAccessTokens{}}}
	}
	return s
}

// RegisterMiniApp 是幂等的：code 只能用一次，小程序只调这一个接口，新老客户都直接拿到 token。
func (s *CustomerAuthService) RegisterMiniApp(ctx context.Context, tenantUUID, platform, appID, code string) (TokenPair, Membership, error) {
	app, user, pluginID, err := s.verifyMiniApp(ctx, tenantUUID, platform, appID, code)
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	pair, m, err := s.registerVerified(ctx, pluginID, miniAppIdentityInput(tenantUUID, app, user))
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	if err := s.recordMiniAppMetadata(ctx, pluginID, app, user); err != nil {
		return TokenPair{}, Membership{}, err
	}
	return pair, m, nil
}

// LoginMiniApp 只为已存在的身份签发 token，不创建客户。
func (s *CustomerAuthService) LoginMiniApp(ctx context.Context, tenantUUID, platform, appID, code string) (TokenPair, Membership, error) {
	app, user, pluginID, err := s.verifyMiniApp(ctx, tenantUUID, platform, appID, code)
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	pair, m, err := s.loginVerified(ctx, pluginID, miniAppIdentityInput(tenantUUID, app, user))
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	if err := s.recordMiniAppMetadata(ctx, pluginID, app, user); err != nil {
		return TokenPair{}, Membership{}, err
	}
	return pair, m, nil
}

// resolveClientApp：插件 STS → 注册表查应用 → 插件 / 租户授权 → 平台有登录校验器 → 密钥已配置。
func (s *CustomerAuthService) resolveClientApp(ctx context.Context, tenantUUID, platform, appID string) (ClientApp, string, error) {
	pluginID, err := stsPluginID(ctx)
	if err != nil {
		return ClientApp{}, "", err
	}
	if s.clientApps == nil {
		return ClientApp{}, "", ErrCustomerClientAppNotFound
	}
	app, ok, lookupErr := s.clientApps.Lookup(ctx, tenantUUID, pluginID, platform, appID)
	if lookupErr != nil {
		return ClientApp{}, "", lookupErr
	}
	if !ok {
		return ClientApp{}, "", ErrCustomerClientAppNotFound
	}
	if !app.allows(pluginID, tenantUUID) {
		return ClientApp{}, "", ErrCustomerClientAppForbidden
	}
	if _, ok := s.verifiers[app.Platform]; !ok {
		return ClientApp{}, "", ErrCustomerClientPlatformUnsupported
	}
	if app.Secret == "" {
		return ClientApp{}, "", ErrCustomerClientAppNotConfigured
	}
	return app, pluginID, nil
}

// verifyMiniApp：解析客户端应用后，用平台校验器校验登录 code。
func (s *CustomerAuthService) verifyMiniApp(ctx context.Context, tenantUUID, platform, appID, code string) (ClientApp, MiniAppUser, string, error) {
	app, pluginID, err := s.resolveClientApp(ctx, tenantUUID, platform, appID)
	if err != nil {
		return ClientApp{}, MiniAppUser{}, "", err
	}
	verifier := s.verifiers[app.Platform]
	code = strings.TrimSpace(code)
	if code == "" {
		return ClientApp{}, MiniAppUser{}, "", ErrCustomerCredentialInvalid
	}
	user, err := verifier.Verify(ctx, app, code)
	if err != nil {
		return ClientApp{}, MiniAppUser{}, "", err
	}
	return app, user, pluginID, nil
}

func miniAppIdentityInput(tenantUUID string, app ClientApp, user MiniAppUser) ResolveExternalIdentityInput {
	// code 校验不返回昵称，先用稳定的占位名，客户之后可自行修改。
	label := user.OpenID
	if len(label) > 4 {
		label = label[len(label)-4:]
	}
	return ResolveExternalIdentityInput{TenantUUID: tenantUUID, ProviderSubject: MiniAppExternalIdentitySubject(app.Platform, app.AppID, user.OpenID), DisplayName: miniAppDisplayPrefix(app.Platform) + label}
}

func miniAppDisplayPrefix(platform string) string {
	switch platform {
	case AlipayMiniAppChannel:
		return "支付宝用户"
	case DouyinMiniAppChannel:
		return "抖音用户"
	default:
		return "微信用户"
	}
}

// recordMiniAppMetadata 在身份 metadata 上记录来源应用；有 unionid 时一并记录，
// 便于以后在同一开放平台下的多个应用之间关联同一客户。
func (s *CustomerAuthService) recordMiniAppMetadata(ctx context.Context, pluginID string, app ClientApp, user MiniAppUser) error {
	provider, err := externalProviderKey(pluginID)
	if err != nil {
		return err
	}
	subject := MiniAppExternalIdentitySubject(app.Platform, app.AppID, user.OpenID)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identity modelcustomer.AuthIdentity
		if err := tx.Where("provider = ? AND provider_subject = ?", provider, subject).First(&identity).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCustomerIdentityNotFound
			}
			return ErrCustomerUpstreamDependency
		}
		meta := map[string]any{}
		if len(identity.Metadata) > 0 {
			_ = json.Unmarshal(identity.Metadata, &meta)
		}
		next := map[string]any{"client_platform": app.Platform, "client_app_id": app.AppID}
		if user.UnionID != "" {
			next[unionIDMetadataKey(app.Platform)] = user.UnionID
		}
		changed := false
		for k, v := range next {
			if meta[k] != v {
				meta[k], changed = v, true
			}
		}
		if !changed {
			return nil
		}
		raw, _ := json.Marshal(meta)
		if err := tx.Model(&modelcustomer.AuthIdentity{}).Where("uuid = ?", identity.UUID).Update("metadata", datatypes.JSON(raw)).Error; err != nil {
			return ErrCustomerUpstreamDependency
		}
		return nil
	})
}

func unionIDMetadataKey(platform string) string {
	return strings.TrimSuffix(platform, "_miniapp") + "_unionid"
}

// MiniAppExternalIdentitySubject 以 <平台>:miniapp:<appid>:<openid> 作为客户身份主键（微信为 wechat:miniapp:...）。
func MiniAppExternalIdentitySubject(platform, appID, openID string) string {
	return strings.TrimSuffix(strings.TrimSpace(platform), "_miniapp") + ":miniapp:" + strings.TrimSpace(appID) + ":" + strings.TrimSpace(openID)
}

// WeChatMiniAppExternalIdentitySubject 是微信小程序的身份主键。
func WeChatMiniAppExternalIdentitySubject(appID, openID string) string {
	return MiniAppExternalIdentitySubject(WeChatMiniAppChannel, appID, openID)
}

// weChatCode2Session 是微信小程序的 code 校验器（jscode2session）。
type weChatCode2Session struct {
	service *CustomerAuthService
}

func (v weChatCode2Session) Verify(ctx context.Context, app ClientApp, code string) (MiniAppUser, error) {
	query := url.Values{"appid": {app.AppID}, "secret": {app.Secret}, "js_code": {code}, "grant_type": {"authorization_code"}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, weChatCode2SessionURL+"?"+query.Encode(), nil)
	resp, err := v.service.client.Do(req)
	if err != nil {
		return MiniAppUser{}, ErrCustomerUpstreamDependency
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return MiniAppUser{}, ErrCustomerUpstreamDependency
	}
	// 微信以 text/plain 返回 JSON；errcode != 0 表示 code 无效、已使用、过期，或与该 AppID 不匹配。
	var out struct {
		OpenID  string `json:"openid"`
		UnionID string `json:"unionid"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return MiniAppUser{}, ErrCustomerUpstreamDependency
	}
	if out.ErrCode != 0 || strings.TrimSpace(out.OpenID) == "" {
		return MiniAppUser{}, ErrCustomerCredentialInvalid
	}
	return MiniAppUser{OpenID: strings.TrimSpace(out.OpenID), UnionID: strings.TrimSpace(out.UnionID)}, nil
}
