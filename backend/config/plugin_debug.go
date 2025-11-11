package config

// PluginDebugConfig captures runtime options for the plugin debug toolchain.
type PluginDebugConfig struct {
	Component     string                         `yaml:"component"`
	HostSimulator PluginDebugHostSimulatorConfig `yaml:"host_simulator"`
	Reports       PluginDebugReportConfig        `yaml:"reports"`
	TicketBridge  PluginDebugTicketBridgeConfig  `yaml:"ticket_bridge"`
	Sandbox       PluginDebugSandboxConfig       `yaml:"sandbox"`
}

// PluginDebugHostSimulatorConfig describes host simulator feature flags.
type PluginDebugHostSimulatorConfig struct {
	Enabled     bool   `yaml:"enabled"`
	FeatureFlag string `yaml:"feature_flag"`
	ConfigPath  string `yaml:"config_path"`
}

// PluginDebugReportConfig describes report template and masking files.
type PluginDebugReportConfig struct {
	TemplatePath    string `yaml:"template"`
	MaskingRules    string `yaml:"masking_rules"`
	FallbackLogBase string `yaml:"fallback_log_base"`
}

// PluginDebugTicketBridgeConfig governs ticket escalation backend.
type PluginDebugTicketBridgeConfig struct {
	Provider string `yaml:"provider"`
	Endpoint string `yaml:"endpoint"`
	Project  string `yaml:"project"`
}

// PluginDebugSandboxConfig toggles sandbox orchestration.
type PluginDebugSandboxConfig struct {
	Enabled       bool   `yaml:"enabled"`
	FeatureFlag   string `yaml:"feature_flag"`
	DataSuitePath string `yaml:"data_suite_path"`
}

// DefaultPluginDebugConfig provides sane defaults aligned with Phase 10.
func DefaultPluginDebugConfig() PluginDebugConfig {
	return PluginDebugConfig{
		Component: "plugin_debug",
		HostSimulator: PluginDebugHostSimulatorConfig{
			Enabled:     true,
			FeatureFlag: "PX_PLUGIN_HOST_SIMULATOR",
			ConfigPath:  "./config/plugins/debug/host_simulator.yaml",
		},
		Reports: PluginDebugReportConfig{
			TemplatePath:    "./config/plugins/debug/report_template.yaml",
			MaskingRules:    "./config/security/data_masking_rules.yaml",
			FallbackLogBase: "",
		},
		TicketBridge: PluginDebugTicketBridgeConfig{
			Provider: "noop",
			Endpoint: "",
			Project:  "plugin-debug",
		},
		Sandbox: PluginDebugSandboxConfig{
			Enabled:       true,
			FeatureFlag:   "plugin-sandbox-suite",
			DataSuitePath: "./config/plugins/debug/data_suite.yaml",
		},
	}
}
