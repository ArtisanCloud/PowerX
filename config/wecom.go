package config

type WecomConfig struct {

	// 企业微信基础配置
	CorpID       string `yaml:"corp_id" json:"corp_id"`
	WecomAgentID int    `yaml:"weCom_agent_id" json:"weCom_agent_id"`
	WecomSecret  string `yaml:"weCom_secret" json:"weCom_secret"`

	// 企业微信应用配置
	AppCertPublicKey      string `yaml:"app_cert_public_key" json:"app_cert_public_key"`
	AppMessageAesKey      string `yaml:"app_message_aes_key" json:"app_message_aes_key"`
	AppMessageCallbackURL string `yaml:"app_message_callback_url" json:"app_message_callback_url"`
	AppMessageToken       string `yaml:"app_message_token" json:"app_message_token"`
	AppOauthCallbackURL   string `yaml:"app_oauth_callback_url" json:"app_oauth_callback_url"`

	// 企业微信客户联系人配置
	CustomerMessageAesKey      string `yaml:"customer_message_aes_key" json:"customer_message_aes_key"`
	CustomerMessageCallbackURL string `yaml:"customer_message_callback_url" json:"customer_message_callback_url"`
	CustomerMessageToken       string `yaml:"customer_message_token" json:"customer_message_token"`

	// 企业微信内部联系人配置
	EmployeeMessageAesKey      string `yaml:"employee_message_aes_key" json:"employee_message_aes_key"`
	EmployeeMessageCallbackURL string `yaml:"employee_message_callback_url" json:"employee_message_callback_url"`
	EmployeeMessageToken       string `yaml:"employee_message_token" json:"employee_message_token"`
}
