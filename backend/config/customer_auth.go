package config

import (
	"fmt"
	"strings"
)

// 小程序等客户端平台。登记为已知平台的才能出现在 client_apps；
// 是否已有校验器（能真正完成登录）由 customer 服务决定，未实现的平台登录时明确报错。
const (
	ClientPlatformWeChatMiniApp = "wechat_miniapp"
	ClientPlatformAlipayMiniApp = "alipay_miniapp"
	ClientPlatformDouyinMiniApp = "douyin_miniapp"
)

// KnownClientPlatforms 返回可配置的客户端平台。
func KnownClientPlatforms() []string {
	return []string{ClientPlatformWeChatMiniApp, ClientPlatformAlipayMiniApp, ClientPlatformDouyinMiniApp}
}

// CustomerAuthConfig 客户委派登录配置。
type CustomerAuthConfig struct {
	// ClientApps 仅为旧配置显式迁移来源；运行时使用租户独立数据库登记，不回退此配置。
	ClientApps []CustomerClientAppConfig `yaml:"client_apps"`
}

// CustomerClientAppConfig 是一个客户端应用。code 校验由 Core 持有密钥完成，插件只转发一次性 code 和 app_id。
type CustomerClientAppConfig struct {
	Platform string `yaml:"platform"`
	AppID    string `yaml:"app_id"`
	Name     string `yaml:"name"`
	// AppSecret 与 AppSecretEnv 二选一；生产环境推荐 AppSecretEnv（从指定环境变量读取）。
	AppSecret    string   `yaml:"app_secret"`
	AppSecretEnv string   `yaml:"app_secret_env"`
	PluginIDs    []string `yaml:"plugin_ids"`
	// TenantUUIDs 为空表示不限租户；非空时只允许这些租户使用。
	TenantUUIDs []string `yaml:"tenant_uuids"`

	envSecret string
}

// Secret 返回解析后的密钥（配置值或环境变量值）；为空表示尚未配置密钥。
func (a CustomerClientAppConfig) Secret() string {
	if strings.TrimSpace(a.AppSecretEnv) != "" {
		return strings.TrimSpace(a.envSecret)
	}
	return strings.TrimSpace(a.AppSecret)
}

// resolveClientAppSecrets 从 app_secret_env 指定的环境变量读取密钥。
func resolveClientAppSecrets(cfg *Config, getenv func(string) string) {
	for i := range cfg.CustomerAuth.ClientApps {
		if name := strings.TrimSpace(cfg.CustomerAuth.ClientApps[i].AppSecretEnv); name != "" {
			cfg.CustomerAuth.ClientApps[i].envSecret = getenv(name)
		}
	}
}

// Validate 校验注册表结构：平台已知、(platform, app_id) 唯一、必须声明插件、密钥来源不冲突。
// 密钥值为空不算结构错误：该应用登录时会明确返回未配置，便于本地先登记再填密钥。
func (c CustomerAuthConfig) Validate() error {
	known := map[string]bool{}
	for _, p := range KnownClientPlatforms() {
		known[p] = true
	}
	seen := map[string]bool{}
	var problems []string
	for i, app := range c.ClientApps {
		label := fmt.Sprintf("customer_auth.client_apps[%d]", i)
		platform, appID := strings.TrimSpace(app.Platform), strings.TrimSpace(app.AppID)
		if !known[platform] {
			problems = append(problems, fmt.Sprintf("%s.platform 不支持: %q（可选 %s）", label, app.Platform, strings.Join(KnownClientPlatforms(), " / ")))
		}
		if appID == "" {
			problems = append(problems, label+".app_id 不能为空")
		}
		key := platform + "|" + appID
		if seen[key] {
			problems = append(problems, fmt.Sprintf("%s 重复登记 %s/%s", label, platform, appID))
		}
		seen[key] = true
		if len(nonBlank(app.PluginIDs)) == 0 {
			problems = append(problems, label+".plugin_ids 不能为空（必须声明允许使用该应用的插件）")
		}
		if strings.TrimSpace(app.AppSecret) != "" && strings.TrimSpace(app.AppSecretEnv) != "" {
			problems = append(problems, label+" 的 app_secret 与 app_secret_env 只能配置一个")
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return nil
}

func nonBlank(values []string) []string {
	var out []string
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			out = append(out, s)
		}
	}
	return out
}
