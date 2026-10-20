package system_recovery

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/ArtisanCloud/PowerX/config"
	manager "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	snapshots "github.com/ArtisanCloud/PowerX/internal/service/snapshot_ops"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

// refreshPluginSources 在采集前重新枚举，不能沿用 Core 启动时的旧安装清单。
func refreshPluginSources(ctx context.Context, source *snapshots.Config, cfg *config.Config) error {
	if source.ConfigFiles == nil {
		source.ConfigFiles = map[string]string{}
	}
	if source.Artifacts == nil {
		source.Artifacts = map[string]string{}
	}
	for key := range source.ConfigFiles {
		if strings.HasPrefix(key, "plugins/") {
			delete(source.ConfigFiles, key)
		}
	}
	for key := range source.Artifacts {
		if strings.HasPrefix(key, "plugins/") {
			delete(source.Artifacts, key)
		}
	}
	if cfg.Plugin.RegistryFile != "" {
		if _, e := os.Stat(cfg.Plugin.RegistryFile); e == nil {
			source.ConfigFiles["plugins/registry.json"] = cfg.Plugin.RegistryFile
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
	}
	installed := cfg.Plugin.InstalledDir
	ids, e := os.ReadDir(installed)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	source.LifecycleRoots = append(source.LifecycleRoots, installed)
	add := func(logical, file string) error {
		if _, e := os.Lstat(file); errors.Is(e, os.ErrNotExist) {
			return nil
		} else if e != nil {
			return e
		}
		for existing := range source.Artifacts {
			if logical == existing || strings.HasPrefix(logical, existing+"/") {
				return nil
			}
			if strings.HasPrefix(existing, logical+"/") {
				delete(source.Artifacts, existing)
			}
		}
		source.Artifacts[logical] = file
		return nil
	}
	for _, id := range ids {
		if !id.IsDir() {
			continue
		}
		credential := filepath.Join(installed, id.Name(), ".runtime-credentials.json")
		if _, err := os.Lstat(credential); err == nil {
			source.ConfigFiles[filepath.ToSlash(filepath.Join("plugins", id.Name(), ".runtime-credentials.json"))] = credential
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		versions, e := os.ReadDir(filepath.Join(installed, id.Name()))
		if e != nil {
			return e
		}
		for _, version := range versions {
			if !version.IsDir() {
				continue
			}
			base := filepath.Join(installed, id.Name(), version.Name())
			logical := filepath.ToSlash(filepath.Join("plugins", id.Name(), version.Name()))
			for _, rel := range []string{"plugin.yaml", "backend/bin", "web-admin", "frontend", "public", "migrations", "contracts", "plugin.d", "skills"} {
				if e = add(logical+"/"+rel, filepath.Join(base, rel)); e != nil {
					return e
				}
			}
			configRoot := filepath.Join(base, "config")
			if _, e = os.Stat(configRoot); errors.Is(e, os.ErrNotExist) {
				continue
			}
			if e = filepath.WalkDir(configRoot, func(file string, d os.DirEntry, e error) error {
				if e != nil {
					return e
				}
				if d.IsDir() {
					return nil
				}
				rel, e := filepath.Rel(configRoot, file)
				if e != nil {
					return e
				}
				source.ConfigFiles[logical+"/config/"+filepath.ToSlash(rel)] = file
				return nil
			}); e != nil {
				return e
			}
		}
	}
	registry := manager.NewJSONRegistry(cfg.Plugin.RegistryFile)
	if e = registry.Load(ctx); e != nil {
		return e
	}
	for _, plugin := range registry.List(ctx) {
		for _, version := range registry.ListVersions(ctx, plugin.ID) {
			item, ok := registry.GetVersion(ctx, plugin.ID, version)
			if !ok {
				return errors.New("plugin registry version disappeared")
			}
			if item.Runtime.Kind == plugin_mgr.RuntimeKindRemote {
				return errors.New("远程插件未纳入完整恢复")
			}
			base := item.Paths.Root
			logical := filepath.ToSlash(filepath.Join("plugins", plugin.ID, version))
			paths := reflect.ValueOf(item.Paths)
			for i := 0; i < paths.NumField(); i++ {
				name := paths.Type().Field(i).Name
				if name == "Root" || name == "ConfigDir" || name == "HostValuesFile" {
					continue
				}
				file := paths.Field(i).String()
				if file == "" {
					continue
				}
				if !inside(base, file) {
					return errors.New("插件注册路径超出版本目录")
				}
				rel, e := filepath.Rel(base, file)
				if e != nil {
					return e
				}
				if e = add(logical+"/"+filepath.ToSlash(rel), file); e != nil {
					return e
				}
			}
		}
	}
	return nil
}
