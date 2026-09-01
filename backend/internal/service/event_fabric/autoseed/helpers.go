package autoseed

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

// ResolveManifestPath 查找插件目录下的 event_fabric manifest。
func ResolveManifestPath(plugin pm.Plugin) (string, error) {
	var candidates []string
	if cfgDir := strings.TrimSpace(plugin.Paths.ConfigDir); cfgDir != "" {
		candidates = append(candidates, filepath.Join(cfgDir, "event_fabric.yaml"))
	}
	if root := strings.TrimSpace(plugin.Paths.Root); root != "" {
		candidates = append(candidates,
			filepath.Join(root, "config", "event_fabric.yaml"),
			filepath.Join(root, "platform_capabilities", "event_fabric.yaml"),
			filepath.Join(root, "event_fabric.yaml"),
		)
	}

	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}

		info, err := os.Stat(candidate)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", err
		}
		if info.IsDir() {
			continue
		}
		return candidate, nil
	}
	return "", nil
}

// BuildSeedVariables 根据插件安装元数据构造模板变量。
func BuildSeedVariables(plugin pm.Plugin) map[string]string {
	vars := map[string]string{}
	add := func(key, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		vars[key] = value
	}
	add("scope", plugin.InstallMetadata.Scope)
	add("namespace", plugin.InstallMetadata.Namespace)
	add("release_channel", plugin.InstallMetadata.ReleaseChannel)
	add("notes", plugin.InstallMetadata.Notes)

	if len(vars) == 0 {
		return nil
	}
	return vars
}
