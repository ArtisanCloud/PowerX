package config

// PluginBootstrapConfig wires template registry + validation defaults used by plugin bootstrap flows.
type PluginBootstrapConfig struct {
	TemplatesIndex   string   `yaml:"templates_index"`   // YAML registry describing available templates
	DefaultTemplate  string   `yaml:"default_template"`  // fallback template id when CLI doesn't specify
	AllowlistedHosts []string `yaml:"allowlisted_hosts"` // allowed git/registry hosts for validation hints
}

// DefaultPluginBootstrapConfig returns opinionated defaults aligned with docs.
func DefaultPluginBootstrapConfig() PluginBootstrapConfig {
	return PluginBootstrapConfig{
		TemplatesIndex:  defaultBackendConfigPath("plugins/templates/index.yaml"),
		DefaultTemplate: "fullstack-go-nuxt",
		AllowlistedHosts: []string{
			"git.powerx.io",
			"gitlab.internal",
			"github.com",
		},
	}
}
