package config

type WXConfig struct {
	AuthCallbackHost string `yaml:"auth_callback_host" json:"auth_callback_host"`

	// 商户号支付配置
	MchID          string `yaml:"mch_id" json:"mch_id"`
	MchApiV3Key    string `yaml:"mch_api_v3_key" json:"mch_api_v3_key"`
	WXCertPath     string `yaml:"wx_cert_path" json:"wx_cert_path"`
	WXKeyPath      string `yaml:"wx_key_path" json:"wx_key_path"`
	WXPayNotifyURL string `yaml:"wx_pay_notify_url" json:"wx_pay_notify_url"`
	NotifyURL      string `yaml:"notify_url" json:"notify_url"`
}

type WXMiniProgramConfig struct {
	MiniProgramAppID  string `yaml:"miniprogram_app_id" json:"miniprogram_app_id"`
	MiniProgramSecret string `yaml:"miniprogram_secret" json:"miniprogram_secret"`
}
