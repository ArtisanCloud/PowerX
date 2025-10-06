package dto

import "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"

type AdminMenuCategory struct {
	ID       plugin_mgr.MenuKey        `json:"id"`
	Title    string                    `json:"title"`
	Order    int                       `json:"order"`
	Origin   plugin_mgr.MenuOriginType `json:"origin"`   // "system" | "plugin" | "mixed"
	Children []AdminMenuItem           `json:"children"` // 该分类下的一层菜单
}

type AdminMenuItem struct {
	Key   plugin_mgr.MenuKey `json:"id"`
	Title string             `json:"title"`
	Icon  string             `json:"icon,omitempty"`
	// 对前端：path；对后端：仍然用 URL 字段名（兼容你现有引用）
	URL         string                    `json:"path,omitempty"`        // 路径
	Order       int                       `json:"order"`                 // 排序
	Visible     bool                      `json:"visible"`               // 是否可见
	Origin      plugin_mgr.MenuOriginType `json:"origin,omitempty"`      // "system" | "plugin" | "mixed"
	Permissions []string                  `json:"permissions,omitempty"` // 权限
	ParentID    plugin_mgr.MenuKey        `json:"parentId,omitempty"`    // 父菜单ID
	Slot        plugin_mgr.SlotKey        `json:"slot,omitempty"`        // 插件插槽
	Children    []AdminMenuItem           `json:"children,omitempty"`    // 子菜单
	TitleI18n   *MenuI18nLabel            `json:"titleI18n,omitempty"`
}

type MenuI18nLabel struct {
	Namespace string `json:"namespace,omitempty"`
	Key       string `json:"key"`
	Default   string `json:"default,omitempty"`
}

type MenuI18nLocales map[string]MenuI18nNamespaces

type MenuI18nNamespaces map[string]map[string]any

type MenuI18nPackage struct {
	PluginID         string          `json:"pluginId"`
	Format           string          `json:"format,omitempty"`
	DefaultNamespace string          `json:"defaultNamespace,omitempty"`
	Namespaces       []string        `json:"namespaces,omitempty"`
	Locales          MenuI18nLocales `json:"locales"`
}
