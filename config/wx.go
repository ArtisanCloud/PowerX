package config

type WXConfig struct {
	AuthCallbackHost string `yaml:"auth_callback_host" json:"auth_callback_host"`
}

type WXMiniProgramConfig struct {
	MiniProgramAppID  string `yaml:"miniprogram_app_id" json:"miniprogram_app_id"`
	MiniProgramSecret string `yaml:"miniprogram_secret" json:"miniprogram_secret"`
}
