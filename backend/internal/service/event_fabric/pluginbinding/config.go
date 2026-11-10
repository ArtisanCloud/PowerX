package pluginbinding

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	pm "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
)

func Bridge(spec map[string]any) map[string]any {
	runtime, _ := spec["runtime"].(map[string]any)
	bridge, _ := runtime["event_bridge"].(map[string]any)
	return bridge
}

func Declared(p pm.Plugin) bool {
	if p.HostConfig == nil || len(requiredActions(p)) == 0 {
		return false
	}
	root, _ := p.HostConfig.Spec["event_bridge"].(map[string]any)
	for _, b := range []map[string]any{Bridge(p.HostConfig.Spec), root} {
		if _, ok := b["topic_manifest"]; ok {
			return true
		}
		if _, ok := b["topic_bindings"]; ok {
			return true
		}
	}
	return false
}

// ManifestPath 校验两种配置位置，拒绝空值、错误类型、冲突和包外路径。
func ManifestPath(p pm.Plugin) (string, error) {
	if p.HostConfig == nil {
		return "", fmt.Errorf("EVENT_FABRIC_INSTALL_VALUES_REQUIRED")
	}
	rootBridge, _ := p.HostConfig.Spec["event_bridge"].(map[string]any)
	rel := ""
	for _, b := range []map[string]any{Bridge(p.HostConfig.Spec), rootBridge} {
		if raw, ok := b["topic_manifest"]; ok {
			value, ok := raw.(string)
			if !ok || value == "" || value != strings.TrimSpace(value) || filepath.IsAbs(value) {
				return "", fmt.Errorf("EVENT_FABRIC_INSTALL_MANIFEST_INVALID")
			}
			if rel != "" && rel != value {
				return "", fmt.Errorf("EVENT_FABRIC_INSTALL_MANIFEST_CONFLICT")
			}
			rel = value
		}
	}
	if rel == "" {
		return "", nil
	}
	root, err := filepath.EvalSymlinks(p.Paths.Root)
	if err != nil {
		return "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, rel))
	if err != nil {
		return "", fmt.Errorf("EVENT_FABRIC_INSTALL_MANIFEST_INVALID: %w", err)
	}
	inside, err := filepath.Rel(root, path)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("EVENT_FABRIC_INSTALL_MANIFEST_OUTSIDE_PACKAGE")
	}
	return path, nil
}

func requiredActions(p pm.Plugin) map[string]bool {
	out := map[string]bool{}
	for _, id := range p.RequiredCapabilities {
		if id == "com.corex.eventfabric.service_publish" {
			out["publish"] = true
		}
		if id == "com.corex.eventfabric.service_consume" {
			out["subscribe"] = true
		}
	}
	return out
}

// ExplicitActions 双向插件必须逐主题声明方向，不能把两个权限自动并到所有主题上。
func ExplicitActions(p pm.Plugin, bindings map[string]string) (map[string][]string, error) {
	allowed := requiredActions(p)
	out := map[string][]string{}
	raw := map[string]any{}
	if p.HostConfig != nil {
		root, _ := p.HostConfig.Spec["event_bridge"].(map[string]any)
		for _, bridge := range []map[string]any{root, Bridge(p.HostConfig.Spec)} {
			if value, exists := bridge["topic_actions"]; exists {
				values, ok := value.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_TOPIC_ACTIONS_INVALID")
				}
				for key, value := range values {
					if previous, exists := raw[key]; exists && !reflect.DeepEqual(previous, value) {
						return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_TOPIC_ACTIONS_CONFLICT: %s", key)
					}
					raw[key] = value
				}
			}
		}
	}
	for key := range bindings {
		if value, exists := raw[key]; exists {
			list, ok := value.([]any)
			if typed, isTyped := value.([]string); isTyped {
				ok = true
				list = make([]any, len(typed))
				for i, item := range typed {
					list[i] = item
				}
			}
			if !ok {
				return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_TOPIC_ACTIONS_INVALID: %s", key)
			}
			for _, item := range list {
				action, ok := item.(string)
				if !ok || !allowed[action] {
					return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_ACL_ACTION_INVALID: %s", key)
				}
				out[key] = append(out[key], action)
			}
		} else if len(allowed) == 1 {
			for action := range allowed {
				out[key] = []string{action}
			}
		}
		if len(out[key]) == 0 {
			return nil, fmt.Errorf("EVENT_FABRIC_INSTALL_TOPIC_ACTIONS_REQUIRED: %s", key)
		}
	}
	return out, nil
}
