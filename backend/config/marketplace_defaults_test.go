package config

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	agentCfg "github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestMarketplaceDefaultIsPublicHTTPSAPI(t *testing.T) {
	market := GetDefaults().Plugin.Marketplace
	require.Equal(t, "https://marketplace.artisan-cloud.com/api/v1", market.APIRoot, "全新安装必须默认指向官方市场")
	endpoint, err := url.Parse(market.APIRoot)
	require.NoError(t, err)
	require.Equal(t, "https", endpoint.Scheme)
	require.NotEmpty(t, endpoint.Hostname())
	require.Equal(t, "/api/v1", endpoint.Path)
	require.Nil(t, endpoint.User)
	require.Empty(t, market.DownloadToken)
	require.Empty(t, market.EnrollmentToken)
	require.Equal(t, 300, market.HeartbeatSeconds)
}

func TestMarketplaceDistributionTemplatesUseOfficialDefault(t *testing.T) {
	for _, name := range []string{"config_example.yaml", "config_example.prod.yaml"} {
		t.Run(name, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join("..", "etc", name))
			require.NoError(t, err)
			var cfg Config
			require.NoError(t, yaml.Unmarshal(body, &cfg))
			require.Equal(t, DefaultMarketplaceAPIRoot, cfg.Plugin.Marketplace.APIRoot)
		})
	}
}

func TestMarketplaceStartupDefaultsAndOverridePrecedence(t *testing.T) {
	isolateInstanceEnvironment(t)
	t.Setenv("POWERX_MARKETPLACE_API_ROOT", "")
	t.Setenv("POWERX_MARKETPLACE_DOWNLOAD_TOKEN", "")
	t.Setenv("POWERX_MARKETPLACE_ENROLLMENT_TOKEN", "")
	t.Setenv("POWERX_MARKETPLACE_HEARTBEAT_SECONDS", "")
	previous, previousPath, previousAI := GlobalConfig, globalConfigPath, agentCfg.GetGlobalAIConfig()
	t.Cleanup(func() {
		GlobalConfig, globalConfigPath = previous, previousPath
		agentCfg.SetGlobalAIConfig(previousAI)
	})
	defaultRoot := GetDefaults().Plugin.Marketplace.APIRoot
	loaders := map[string]func(string) (*Config, error){
		"Load": Load,
		"InitGlobalConfig": func(path string) (*Config, error) {
			if err := InitGlobalConfig(path); err != nil {
				return nil, err
			}
			return GlobalConfig, nil
		},
	}
	for name, load := range loaders {
		t.Run(name, func(t *testing.T) {
			for _, row := range []struct{ name, yaml, env, want string }{
				{name: "fresh_setup", want: defaultRoot},
				{name: "partial_plugin", yaml: "plugin:\n  enabled: true\n", want: defaultRoot},
				{name: "partial_marketplace", yaml: "plugin:\n  marketplace:\n    heartbeat_seconds: 420\n", want: defaultRoot},
				{name: "config_override", yaml: "plugin:\n  marketplace:\n    api_root: http://127.0.0.1:8088/api/v1\n", want: "http://127.0.0.1:8088/api/v1"},
				{name: "explicit_disable", yaml: "plugin:\n  marketplace:\n    api_root: \"\"\n", want: ""},
				{name: "environment_override", yaml: "plugin:\n  marketplace:\n    api_root: https://private.example/api/v1\n", env: "https://override.example/api/v1", want: "https://override.example/api/v1"},
			} {
				t.Run(row.name, func(t *testing.T) {
					t.Setenv("POWERX_MARKETPLACE_API_ROOT", row.env)
					path := setupSecretTestPath(t, "uninstalled", "\"\"")
					original, err := os.ReadFile(path)
					require.NoError(t, err)
					original = append(original, row.yaml...)
					require.NoError(t, os.WriteFile(path, original, 0600))
					cfg, err := load(path)
					require.NoError(t, err)
					require.Equal(t, row.want, cfg.Plugin.Marketplace.APIRoot)
					after, err := os.ReadFile(path)
					require.NoError(t, err)
					require.Equal(t, original, after, "加载配置不得改写用户的市场地址")
				})
			}
			t.Run("reload_config_override", func(t *testing.T) {
				t.Setenv("POWERX_MARKETPLACE_API_ROOT", "")
				path := setupSecretTestPath(t, "uninstalled", "\"\"")
				base, err := os.ReadFile(path)
				require.NoError(t, err)
				for _, root := range []string{"https://first.example/api/v1", "http://127.0.0.1:8088/api/v1"} {
					body := append([]byte(nil), base...)
					body = append(body, "plugin:\n  marketplace:\n    api_root: "+root+"\n"...)
					require.NoError(t, os.WriteFile(path, body, 0600))
					cfg, err := load(path)
					require.NoError(t, err)
					require.Equal(t, root, cfg.Plugin.Marketplace.APIRoot, "重新加载必须读取运维更新后的地址")
				}
			})
		})
	}
}
