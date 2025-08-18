package plugin_mgr

import "context"

// pkg/plugi_nmgr/contract.go
type Manager interface {
	Bootstrap(ctx context.Context) error
	Shutdown(ctx context.Context) error

	// 安装与升级
	InstallFromFile(ctx context.Context, path string, opts InstallOptions) (Plugin, error)
	InstallFromURL(ctx context.Context, url, sha256, signature string, opts InstallOptions) (Plugin, error)
	Upgrade(ctx context.Context, id, version string, src InstallSource, opts InstallOptions) (Plugin, error)

	// 生命周期
	Enable(ctx context.Context, id string) error
	Disable(ctx context.Context, id string) error
	Uninstall(ctx context.Context, id string, versionOptional ...string) error

	// 查询
	List(ctx context.Context) ([]Plugin, error)
	Get(ctx context.Context, id string) (Plugin, error)
}
