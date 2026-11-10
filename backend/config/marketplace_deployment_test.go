package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMarketplacePublicSiteEnvironmentUnsetAndClear(t *testing.T) {
	cfg := GetDefaults()
	cfg.Plugin.Marketplace.PublicSiteURL = "https://file.example/console/"
	require.NoError(t, loadFromEnvironment(cfg, func(string) (string, bool) { return "", false }))
	require.Equal(t, "https://file.example/console/", cfg.Plugin.Marketplace.PublicSiteURL)
	require.NoError(t, loadFromEnvironment(cfg, func(key string) (string, bool) { return "", key == "POWERX_MARKETPLACE_PUBLIC_SITE_URL" }))
	require.Empty(t, cfg.Plugin.Marketplace.PublicSiteURL)
	require.NoError(t, loadFromEnvironment(cfg, func(key string) (string, bool) {
		if key == "POWERX_MARKETPLACE_PUBLIC_SITE_URL" {
			return "https://override.example/", true
		}
		return "", false
	}))
	require.Equal(t, "https://override.example/", cfg.Plugin.Marketplace.PublicSiteURL)
}
