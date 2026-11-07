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

const (
	WeChatMiniAppChannel        = "wechat_miniapp"
	WeChatMiniAppCredentialType = "wechat_miniapp_code"

	weChatCode2SessionURL = "https://api.weixin.qq.com/sns/jscode2session"
)

// WeChatMiniAppConfig 是平台持有的小程序凭证（Shared App）。
// code2session 由 Core 调用，插件只转发一次性的 wx.login code，无法自称某个 openid。
type WeChatMiniAppConfig struct {
	AppID     string
	AppSecret string
}

type VerifiedWeChatMiniAppUser struct {
	Subject string
	OpenID  string
	UnionID string
}

// WithWeChatMiniApp 启用 wechat_miniapp 渠道。
func (s *CustomerAuthService) WithWeChatMiniApp(cfg WeChatMiniAppConfig) *CustomerAuthService {
	if s != nil {
		s.wechat = WeChatMiniAppConfig{AppID: strings.TrimSpace(cfg.AppID), AppSecret: strings.TrimSpace(cfg.AppSecret)}
	}
	return s
}

// RegisterWeChatMiniApp 是幂等的：wx.login 的 code 只能用一次，
// 小程序只调这一个接口，新老客户都直接拿到 token。
func (s *CustomerAuthService) RegisterWeChatMiniApp(ctx context.Context, tenantUUID, code string) (TokenPair, Membership, error) {
	verified, pluginID, err := s.verifyWeChatMiniApp(ctx, code)
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	pair, m, err := s.registerVerified(ctx, pluginID, weChatIdentityInput(tenantUUID, verified))
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	if err := s.recordWeChatUnionID(ctx, pluginID, verified); err != nil {
		return TokenPair{}, Membership{}, err
	}
	return pair, m, nil
}

// LoginWeChatMiniApp 只为已存在的身份签发 token，不创建客户。
func (s *CustomerAuthService) LoginWeChatMiniApp(ctx context.Context, tenantUUID, code string) (TokenPair, Membership, error) {
	verified, pluginID, err := s.verifyWeChatMiniApp(ctx, code)
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	pair, m, err := s.loginVerified(ctx, pluginID, weChatIdentityInput(tenantUUID, verified))
	if err != nil {
		return TokenPair{}, Membership{}, err
	}
	if err := s.recordWeChatUnionID(ctx, pluginID, verified); err != nil {
		return TokenPair{}, Membership{}, err
	}
	return pair, m, nil
}

func weChatIdentityInput(tenantUUID string, verified VerifiedWeChatMiniAppUser) ResolveExternalIdentityInput {
	// code2session 不返回昵称，先用稳定的占位名，客户之后可自行修改。
	label := verified.OpenID
	if len(label) > 4 {
		label = label[len(label)-4:]
	}
	return ResolveExternalIdentityInput{TenantUUID: tenantUUID, ProviderSubject: verified.Subject, DisplayName: "微信用户" + label}
}

func (s *CustomerAuthService) verifyWeChatMiniApp(ctx context.Context, code string) (VerifiedWeChatMiniAppUser, string, error) {
	pluginID, err := stsPluginID(ctx)
	if err != nil {
		return VerifiedWeChatMiniAppUser{}, "", err
	}
	if s.wechat.AppID == "" || s.wechat.AppSecret == "" {
		return VerifiedWeChatMiniAppUser{}, "", ErrCustomerUpstreamDependency
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return VerifiedWeChatMiniAppUser{}, "", ErrCustomerCredentialInvalid
	}
	query := url.Values{"appid": {s.wechat.AppID}, "secret": {s.wechat.AppSecret}, "js_code": {code}, "grant_type": {"authorization_code"}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, weChatCode2SessionURL+"?"+query.Encode(), nil)
	resp, err := s.client.Do(req)
	if err != nil {
		return VerifiedWeChatMiniAppUser{}, "", ErrCustomerUpstreamDependency
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return VerifiedWeChatMiniAppUser{}, "", ErrCustomerUpstreamDependency
	}
	// 微信以 text/plain 返回 JSON；errcode != 0 表示 code 无效、已使用或过期。
	var out struct {
		OpenID  string `json:"openid"`
		UnionID string `json:"unionid"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return VerifiedWeChatMiniAppUser{}, "", ErrCustomerUpstreamDependency
	}
	if out.ErrCode != 0 || strings.TrimSpace(out.OpenID) == "" {
		return VerifiedWeChatMiniAppUser{}, "", ErrCustomerCredentialInvalid
	}
	openID := strings.TrimSpace(out.OpenID)
	return VerifiedWeChatMiniAppUser{Subject: WeChatMiniAppExternalIdentitySubject(s.wechat.AppID, openID), OpenID: openID, UnionID: strings.TrimSpace(out.UnionID)}, pluginID, nil
}

// recordWeChatUnionID 把 unionid 记在以 openid 为键的身份 metadata 上，
// 便于以后在同一开放平台下的多个应用之间关联同一客户。
func (s *CustomerAuthService) recordWeChatUnionID(ctx context.Context, pluginID string, verified VerifiedWeChatMiniAppUser) error {
	if verified.UnionID == "" {
		return nil
	}
	provider, err := externalProviderKey(pluginID)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var identity modelcustomer.AuthIdentity
		if err := tx.Where("provider = ? AND provider_subject = ?", provider, verified.Subject).First(&identity).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCustomerIdentityNotFound
			}
			return ErrCustomerUpstreamDependency
		}
		meta := map[string]any{}
		if len(identity.Metadata) > 0 {
			_ = json.Unmarshal(identity.Metadata, &meta)
		}
		if meta["wechat_unionid"] == verified.UnionID {
			return nil
		}
		meta["wechat_unionid"] = verified.UnionID
		raw, _ := json.Marshal(meta)
		if err := tx.Model(&modelcustomer.AuthIdentity{}).Where("uuid = ?", identity.UUID).Update("metadata", datatypes.JSON(raw)).Error; err != nil {
			return ErrCustomerUpstreamDependency
		}
		return nil
	})
}

// WeChatMiniAppExternalIdentitySubject 以小程序 openid 作为客户身份主键。
func WeChatMiniAppExternalIdentitySubject(appID, openID string) string {
	return "wechat:miniapp:" + strings.TrimSpace(appID) + ":" + strings.TrimSpace(openID)
}
