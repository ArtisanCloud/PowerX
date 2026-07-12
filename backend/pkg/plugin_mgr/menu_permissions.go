package plugin_mgr

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	MenuPermissionModule = "menu"
	MenuPermissionAction = "read"
)

var menuPermissionSegmentRE = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

func PluginMenuPermissionResource(pluginID string, item MenuItem) string {
	pluginID = strings.TrimSpace(pluginID)
	segment := PluginMenuPermissionSegment(item)
	if pluginID == "" || segment == "" {
		return ""
	}
	return "plugin." + pluginID + "." + segment
}

func PluginMenuPermissionPolicy(pluginID string, item MenuItem) string {
	resource := PluginMenuPermissionResource(pluginID, item)
	if resource == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s:%s", MenuPermissionModule, resource, MenuPermissionAction)
}

func PluginMenuPermissionSegment(item MenuItem) string {
	candidates := []string{
		strings.TrimSpace(item.ID),
		strings.Trim(strings.TrimSpace(item.Route), "/"),
		strings.Trim(strings.TrimSpace(item.Path), "/"),
	}
	for _, candidate := range candidates {
		if segment := normalizeMenuPermissionSegment(candidate); segment != "" {
			return segment
		}
	}
	if item.Order != 0 {
		return fmt.Sprintf("order.%d", item.Order)
	}
	return ""
}

func normalizeMenuPermissionSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "/", ".")
	value = menuPermissionSegmentRE.ReplaceAllString(value, ".")
	value = strings.Trim(value, ".-_")
	for strings.Contains(value, "..") {
		value = strings.ReplaceAll(value, "..", ".")
	}
	return value
}
