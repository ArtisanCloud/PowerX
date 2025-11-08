// api/http/admin/dto/plugin_view.go  —— 仅当需要 Admin 特有字段时才放
package dto

import base "github.com/ArtisanCloud/PowerX/pkg/dto/plugin_mgr"

type PluginItemView struct {
	base.PluginItem // 嵌入共享字段
	// Admin 专属扩展（示例，可按需添加）
	// Permissions []string `json:"permissions,omitempty"`
}

type PluginMenuItem = base.PluginMenuItem
