package plugin_mgr

// InstallPreview 只包含安装确认所需的信息，不能携带配置或运行凭证。
type InstallPreview struct {
	Version  string           `json:"version"`
	Revision string           `json:"revision,omitempty"`
	Provider ProviderIdentity `json:"provider"`
	Origin   InstallOrigin    `json:"origin"`
}

// ReplacementRequiredError 要求安装入口先展示目标和原安装，再带修订号确认替换。
type ReplacementRequiredError struct {
	RequiresProviderConfirmation bool           `json:"requires_provider_confirmation"`
	PluginID                     string         `json:"plugin_id"`
	Current                      InstallPreview `json:"current"`
	Target                       InstallPreview `json:"target"`
}

func (e *ReplacementRequiredError) Error() string {
	return "插件已安装，请查看当前版本与目标包，并确认替换当前安装"
}
func (e *ReplacementRequiredError) Unwrap() error {
	return NewError(CodeConflict, WithOp("PLUGIN_REPLACEMENT_REQUIRED"), WithPlugin(e.PluginID), WithMsg("%s", e.Error()))
}
