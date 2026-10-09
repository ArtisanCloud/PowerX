package bootstrap

import (
	"testing"

	"github.com/ArtisanCloud/PowerX/config"
	"github.com/stretchr/testify/require"
)

func TestPluginRuntimeGatewayBaseUsesInternalAddressBeforePublicAddress(t *testing.T) {
	t.Setenv("POWERX_INTERNAL_GATEWAY_BASE_URL", "")
	t.Setenv("POWERX_HTTP_PROXY_BASE", "http://127.0.0.1:8080/")
	t.Setenv("POWERX_GATEWAY_BASE_URL", "https://expired-public.example")
	require.Equal(t, "http://127.0.0.1:8080", resolvePluginRuntimeGatewayBaseURL(&config.Config{
		Server: config.ServerConfig{Host: "127.0.0.1", Port: 8080},
	}))
}

func TestPluginRuntimeGatewayBaseRespectsExplicitInternalOverride(t *testing.T) {
	t.Setenv("POWERX_INTERNAL_GATEWAY_BASE_URL", "http://core.internal:8080/")
	t.Setenv("POWERX_HTTP_PROXY_BASE", "http://127.0.0.1:8080")
	t.Setenv("POWERX_GATEWAY_BASE_URL", "https://public.example")
	require.Equal(t, "http://core.internal:8080", resolvePluginRuntimeGatewayBaseURL(nil))
}

func TestPluginRuntimeGatewayBaseUsesCoreListenerWhenNoOverrideExists(t *testing.T) {
	t.Setenv("POWERX_INTERNAL_GATEWAY_BASE_URL", "")
	t.Setenv("POWERX_HTTP_PROXY_BASE", "")
	t.Setenv("POWERX_GATEWAY_BASE_URL", "")
	require.Equal(t, "http://127.0.0.1:8080", resolvePluginRuntimeGatewayBaseURL(&config.Config{
		Server: config.ServerConfig{Host: "0.0.0.0", Port: 8080},
	}))
}
