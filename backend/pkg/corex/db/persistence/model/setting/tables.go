// pkg/corex/db/persistence/model/setting/tables.go
package setting

const (
	// 配置域（cfg）
	TableSystemSetting = "cfg_sys_settings"
	TableTenantSetting = "cfg_tenant_settings"
	TableDomainBinding = "cfg_domain_bindings"
	TableTLSCertRef    = "cfg_tls_cert_refs"

	// 身份域（iam）
	TableAuthProviderConfig = "iam_auth_provider_configs"

	// 插件域（plugin）
	TablePluginInstanceConfig = "plugin_instance_configs"
	TablePluginDrainJob       = "plugin_drain_jobs"

	// 可选：枚举
	HTTPSModeAuto    = "auto"   // ACME/Let's Encrypt
	HTTPSModeManual  = "manual" // 手动上传
	HTTPSModeDisable = "disable"

	AuthProviderBuiltin = "builtin"
	AuthProviderOIDC    = "oidc"
	AuthProviderSAML    = "saml"
	AuthProviderLDAP    = "ldap"
	AuthProviderWechat  = "wechat" // 示例：企业微信/OAuth2
)
