package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ArtisanCloud/PowerX/config"
	mgrimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func absPath(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	wd, err := os.Getwd()
	if err != nil {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(wd, p))
}

func loadRegistrySnapshot(ctx context.Context) (*mgrimpl.JSONRegistry, error) {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return nil, fmt.Errorf("global config is nil")
	}
	registryPath := strings.TrimSpace(cfg.Plugin.RegistryFile)
	if registryPath == "" {
		return nil, fmt.Errorf("plugin registry file is empty")
	}
	reg := mgrimpl.NewJSONRegistry(absPath(registryPath))
	if err := reg.Load(ctx); err != nil {
		return nil, err
	}
	return reg, nil
}

func listPluginsFromRegistry(ctx context.Context) ([]plugin_mgr.Plugin, error) {
	reg, err := loadRegistrySnapshot(ctx)
	if err != nil {
		return nil, err
	}
	regItems := reg.List(ctx)

	// 合并磁盘残留（registry 为空或丢失时仍可展示并清理）
	diskItems, _ := listPluginsFromInstalledDir()
	if len(diskItems) == 0 {
		return regItems, nil
	}
	merged := make(map[string]plugin_mgr.Plugin, len(regItems)+len(diskItems))
	for _, p := range regItems {
		merged[p.ID] = p
	}
	for _, p := range diskItems {
		if _, ok := merged[p.ID]; !ok {
			merged[p.ID] = p
		}
	}
	out := make([]plugin_mgr.Plugin, 0, len(merged))
	for _, p := range merged {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func getPluginFromRegistry(ctx context.Context, id string) (plugin_mgr.Plugin, error) {
	reg, err := loadRegistrySnapshot(ctx)
	if err != nil {
		return plugin_mgr.Plugin{}, err
	}
	p, ok := reg.Get(ctx, id)
	if !ok {
		// registry 不存在时，回退到磁盘残留
		diskItems, _ := listPluginsFromInstalledDir()
		for _, item := range diskItems {
			if strings.EqualFold(strings.TrimSpace(item.ID), strings.TrimSpace(id)) {
				return item, nil
			}
		}
		return plugin_mgr.Plugin{}, plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithPlugin(id), plugin_mgr.WithOp("registry_get"))
	}
	return p, nil
}

func uninstallFromRegistry(ctx context.Context, id, version string, purge bool) error {
	reg, err := loadRegistrySnapshot(ctx)
	if err != nil {
		return err
	}

	targetVersion := strings.TrimSpace(version)
	if targetVersion == "" {
		if cur, ok := reg.CurrentVersion(ctx, id); ok && strings.TrimSpace(cur) != "" {
			targetVersion = cur
		} else if p, ok := reg.Get(ctx, id); ok {
			targetVersion = strings.TrimSpace(p.Version)
		}
	}
	if targetVersion == "" {
		return plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithPlugin(id), plugin_mgr.WithOp("registry_uninstall"))
	}

	p, ok := reg.GetVersion(ctx, id, targetVersion)
	if !ok {
		// registry 中没有版本，尝试直接清理磁盘残留
		return uninstallResidualFromDisk(id, targetVersion, purge)
	}
	if purge {
		root := strings.TrimSpace(p.Paths.Root)
		if root != "" {
			_ = os.RemoveAll(root)
		}
	}
	if err := reg.Remove(ctx, id, targetVersion); err != nil {
		return err
	}
	if err := reg.Save(ctx); err != nil {
		return err
	}
	return nil
}

func listPluginsFromInstalledDir() ([]plugin_mgr.Plugin, error) {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return nil, fmt.Errorf("global config is nil")
	}
	installedRoot := strings.TrimSpace(cfg.Plugin.InstalledDir)
	if installedRoot == "" {
		return []plugin_mgr.Plugin{}, nil
	}
	root := absPath(installedRoot)
	pluginDirs, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []plugin_mgr.Plugin{}, nil
		}
		return nil, err
	}

	items := make([]plugin_mgr.Plugin, 0, len(pluginDirs))
	for _, idDir := range pluginDirs {
		if !idDir.IsDir() {
			continue
		}
		pluginID := strings.TrimSpace(idDir.Name())
		if pluginID == "" {
			continue
		}
		versionDirs, err := os.ReadDir(filepath.Join(root, pluginID))
		if err != nil {
			continue
		}
		type verItem struct {
			version string
			root    string
			modTime time.Time
		}
		candidates := make([]verItem, 0, len(versionDirs))
		for _, vDir := range versionDirs {
			if !vDir.IsDir() {
				continue
			}
			ver := strings.TrimSpace(vDir.Name())
			if ver == "" {
				continue
			}
			verRoot := filepath.Join(root, pluginID, ver)
			if _, statErr := os.Stat(filepath.Join(verRoot, "plugin.yaml")); statErr != nil {
				continue
			}
			info, _ := vDir.Info()
			modAt := time.Time{}
			if info != nil {
				modAt = info.ModTime()
			}
			candidates = append(candidates, verItem{
				version: ver,
				root:    verRoot,
				modTime: modAt,
			})
		}
		if len(candidates) == 0 {
			continue
		}
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].modTime.After(candidates[j].modTime)
		})
		cur := candidates[0]
		items = append(items, plugin_mgr.Plugin{
			ID:      pluginID,
			Name:    pluginID,
			Version: cur.version,
			State:   plugin_mgr.PluginState("broken"),
			Paths: plugin_mgr.InstalledPaths{
				Root: cur.root,
			},
		})
	}
	return items, nil
}

func uninstallResidualFromDisk(id, version string, purge bool) error {
	cfg := config.GetGlobalConfig()
	if cfg == nil {
		return fmt.Errorf("global config is nil")
	}
	root := absPath(strings.TrimSpace(cfg.Plugin.InstalledDir))
	if root == "" {
		return plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(version), plugin_mgr.WithOp("registry_uninstall"))
	}
	target := filepath.Join(root, id, version)
	if _, err := os.Stat(target); err != nil {
		return plugin_mgr.NewError(plugin_mgr.CodeNotFound, plugin_mgr.WithPlugin(id), plugin_mgr.WithVersion(version), plugin_mgr.WithOp("registry_uninstall"))
	}
	// 对于残留目录，卸载语义即磁盘清理；purge 开关保持兼容，不再区分
	if purge || !purge {
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		_ = os.Remove(filepath.Dir(target))
	}
	return nil
}
