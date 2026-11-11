package plugin_mgr

import "context"

type expectedInstallationKey struct{}
type expectedInstallation struct{ Version, Revision string }

func WithExpectedInstallation(ctx context.Context, version, revision string) context.Context {
	return context.WithValue(ctx, expectedInstallationKey{}, expectedInstallation{Version: version, Revision: revision})
}
func ExpectedInstallation(ctx context.Context) (version, revision string, present bool) {
	value, ok := ctx.Value(expectedInstallationKey{}).(expectedInstallation)
	return value.Version, value.Revision, ok
}

// InstallCompletionError 表示安装产物已提交，仍需显式完成激活或租户配置。
// 只携带可公开的安装身份；不得将 HostConfig 或运行凭据写入错误回包。
type InstallCompletionError struct {
	PluginID string
	Version  string
	Revision string
	State    PluginState
	Stage    string
	Cause    error
}

func (e *InstallCompletionError) Error() string {
	return "PLUGIN_INSTALL_FINALIZATION_REQUIRED: 插件已安装，尚未完成启用配置"
}
func (e *InstallCompletionError) Unwrap() error { return e.Cause }

func PendingInstallation(p Plugin, stage string, cause error) *InstallCompletionError {
	return &InstallCompletionError{PluginID: p.ID, Version: p.Version, Revision: p.Revision, State: p.State, Stage: stage, Cause: cause}
}
