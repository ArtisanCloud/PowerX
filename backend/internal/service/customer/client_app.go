package customer

import (
	"context"
	"errors"
	"strings"
)

// 小程序等客户端平台（委派登录的 channel 即平台名）。每个平台对应一种一次性 code 凭证。
const (
	WeChatMiniAppChannel = "wechat_miniapp"
	AlipayMiniAppChannel = "alipay_miniapp"
	DouyinMiniAppChannel = "douyin_miniapp"

	WeChatMiniAppCredentialType = "wechat_miniapp_code"
	AlipayMiniAppCredentialType = "alipay_miniapp_code"
	DouyinMiniAppCredentialType = "douyin_miniapp_code"
)

var miniAppCredentialTypes = map[string]string{
	WeChatMiniAppChannel: WeChatMiniAppCredentialType,
	AlipayMiniAppChannel: AlipayMiniAppCredentialType,
	DouyinMiniAppChannel: DouyinMiniAppCredentialType,
}

// MiniAppCredentialType 返回平台对应的 code 凭证类型；不是小程序平台时 ok=false。
func MiniAppCredentialType(platform string) (string, bool) {
	t, ok := miniAppCredentialTypes[strings.TrimSpace(platform)]
	return t, ok
}

var (
	ErrCustomerClientAppNotFound         = errors.New("customer.client_app_not_found")
	ErrCustomerClientAppForbidden        = errors.New("customer.client_app_forbidden")
	ErrCustomerClientAppNotConfigured    = errors.New("customer.client_app_not_configured")
	ErrCustomerClientPlatformUnsupported = errors.New("customer.client_platform_unsupported")
)

// ClientApp 是客户端应用注册表中的一项：凭证由 Core 持有，只有登记的插件（与可选的租户）可以使用。
type ClientApp struct {
	Platform          string
	AppID             string
	Name              string
	Secret            string
	PluginIDs         []string
	TenantUUIDs       []string
	CredentialVersion int64
	CredentialScope   string
}

func (a ClientApp) allows(pluginID, tenantUUID string) bool {
	return containsTrimmed(a.PluginIDs, pluginID) && (len(a.TenantUUIDs) == 0 || containsTrimmed(a.TenantUUIDs, tenantUUID))
}

func containsTrimmed(list []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, v := range list {
		if strings.EqualFold(strings.TrimSpace(v), want) {
			return true
		}
	}
	return false
}

// ClientAppRegistry 在已验证的租户和插件作用域中查找客户端应用。
type ClientAppRegistry interface {
	Lookup(ctx context.Context, tenantUUID, pluginID, platform, appID string) (ClientApp, bool, error)
}

// StaticClientAppRegistry 是基于配置的注册表。
type StaticClientAppRegistry struct {
	apps map[string]ClientApp
}

func NewStaticClientAppRegistry(apps []ClientApp) *StaticClientAppRegistry {
	r := &StaticClientAppRegistry{apps: map[string]ClientApp{}}
	for _, app := range apps {
		app.Platform, app.AppID, app.Secret = strings.TrimSpace(app.Platform), strings.TrimSpace(app.AppID), strings.TrimSpace(app.Secret)
		r.apps[app.Platform+"|"+app.AppID] = app
	}
	return r
}

func (r *StaticClientAppRegistry) Lookup(ctx context.Context, tenantUUID, pluginID, platform, appID string) (ClientApp, bool, error) {
	if r == nil {
		return ClientApp{}, false, nil
	}
	app, ok := r.apps[strings.TrimSpace(platform)+"|"+strings.TrimSpace(appID)]
	if ok && !app.allows(pluginID, tenantUUID) {
		return ClientApp{}, false, ErrCustomerClientAppForbidden
	}
	return app, ok, nil
}
