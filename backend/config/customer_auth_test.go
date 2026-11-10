package config

import (
	"strings"
	"testing"
)

func TestCustomerClientAppsValidate(t *testing.T) {
	ok := CustomerClientAppConfig{Platform: ClientPlatformWeChatMiniApp, AppID: "wx1", PluginIDs: []string{"com.powerx.plugins.scora"}}
	for _, tc := range []struct {
		name string
		apps []CustomerClientAppConfig
		want string
	}{
		{"合法：密钥可以暂缺", []CustomerClientAppConfig{ok}, ""},
		{"预留平台可以登记", []CustomerClientAppConfig{ok, {Platform: ClientPlatformAlipayMiniApp, AppID: "2021", PluginIDs: []string{"p"}}}, ""},
		{"未知平台", []CustomerClientAppConfig{{Platform: "qq_miniapp", AppID: "q1", PluginIDs: []string{"p"}}}, "platform 不支持"},
		{"缺 app_id", []CustomerClientAppConfig{{Platform: ClientPlatformWeChatMiniApp, PluginIDs: []string{"p"}}}, "app_id 不能为空"},
		{"重复登记", []CustomerClientAppConfig{ok, ok}, "重复登记"},
		{"缺插件", []CustomerClientAppConfig{{Platform: ClientPlatformWeChatMiniApp, AppID: "wx2", PluginIDs: []string{" "}}}, "plugin_ids 不能为空"},
		{"密钥来源冲突", []CustomerClientAppConfig{{Platform: ClientPlatformWeChatMiniApp, AppID: "wx3", PluginIDs: []string{"p"}, AppSecret: "s", AppSecretEnv: "X"}}, "只能配置一个"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CustomerAuthConfig{ClientApps: tc.apps}.Validate()
			if tc.want == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("error = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func TestCustomerClientAppSecretFromEnvironment(t *testing.T) {
	cfg := &Config{CustomerAuth: CustomerAuthConfig{ClientApps: []CustomerClientAppConfig{
		{Platform: ClientPlatformWeChatMiniApp, AppID: "wx-env", AppSecretEnv: "TEST_SCORA_WECHAT_SECRET", PluginIDs: []string{"p"}},
		{Platform: ClientPlatformWeChatMiniApp, AppID: "wx-yaml", AppSecret: " yaml-secret ", PluginIDs: []string{"p"}},
		{Platform: ClientPlatformWeChatMiniApp, AppID: "wx-missing", AppSecretEnv: "TEST_MISSING_SECRET", PluginIDs: []string{"p"}},
	}}}
	env := map[string]string{"TEST_SCORA_WECHAT_SECRET": "env-secret"}
	resolveClientAppSecrets(cfg, func(name string) string { return env[name] })
	apps := cfg.CustomerAuth.ClientApps
	if apps[0].Secret() != "env-secret" || apps[1].Secret() != "yaml-secret" || apps[2].Secret() != "" {
		t.Fatalf("secrets = %q %q %q", apps[0].Secret(), apps[1].Secret(), apps[2].Secret())
	}
}
