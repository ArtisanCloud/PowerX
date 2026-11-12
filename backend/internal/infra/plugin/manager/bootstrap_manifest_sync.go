package manager

import (
	"context"
	"sort"

	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// syncBootstrapManifests 将磁盘发现与生效声明同步分开。权限和 required
// capability grants 都按插件共享，不能用每个历史版本依次覆盖。
func (m *managerImpl) syncBootstrapManifests(ctx context.Context, descs []Descriptor) {
	if m.opts.PostInstallManifest == nil {
		return
	}
	manifests := make(map[string]map[string]plugin_mgr.Manifest)
	var ids []string
	for _, desc := range descs {
		id, version := desc.Manifest.ID, desc.Manifest.Version
		if manifests[id] == nil {
			manifests[id] = make(map[string]plugin_mgr.Manifest)
			ids = append(ids, id)
		}
		manifests[id][version] = desc.Manifest
	}
	sort.Strings(ids)
	for _, id := range ids {
		version, selected := m.opts.Registry.CurrentVersion(ctx, id)
		if !selected {
			versions := m.opts.Registry.ListVersions(ctx, id)
			// 单版本安装仍可补齐登记；多版本没有 current 时不猜选最新版本，
			// 也不使用被本次目录扫描改写的 installed_at 作为授权依据。
			if len(versions) != 1 {
				logger.WarnF(ctx, "[plugin-bootstrap] skip manifest sync: no selected version id=%s versions=%v; select a version before syncing permissions", id, versions)
				continue
			}
			version = versions[0]
		}
		manifest, found := manifests[id][version]
		if !found {
			// 当前包缺失或校验失败时，不能退回历史包或注册表中的旧快照。
			logger.WarnF(ctx, "[plugin-bootstrap] skip manifest sync: selected package unavailable id=%s ver=%s", id, version)
			continue
		}
		if err := m.opts.PostInstallManifest(ctx, manifest); err != nil {
			wrapped := plugin_mgr.Wrap(plugin_mgr.CodeInternal, err,
				plugin_mgr.WithOp("bootstrap.register_permissions"),
				plugin_mgr.WithPlugin(id),
				plugin_mgr.WithVersion(version),
			)
			logger.WarnF(ctx, "[plugin-bootstrap] plugin permission sync failed, keep manager available: id=%s ver=%s err=%v", id, version, wrapped)
		}
	}
}
