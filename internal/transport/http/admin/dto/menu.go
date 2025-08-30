package dto

type AdminMenuCategory struct {
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Order    int             `json:"order"`
	Origin   string          `json:"origin"`   // "system" | "plugin" | "mixed"
	Children []AdminMenuItem `json:"children"` // 该分类下的一层菜单
}

type AdminMenuItem struct {
	Key   string `json:"id"`
	Title string `json:"title"`
	Icon  string `json:"icon,omitempty"`
	// 对前端：path；对后端：仍然用 URL 字段名（兼容你现有引用）
	URL         string          `json:"path,omitempty"`        // 路径
	Order       int             `json:"order"`                 // 排序
	Visible     bool            `json:"visible"`               // 是否可见
	Origin      string          `json:"origin,omitempty"`      // "system" | "plugin" | "mixed"
	Permissions []string        `json:"permissions,omitempty"` // 权限
	ParentID    string          `json:"parentId,omitempty"`    // 父菜单ID
	Slot        string          `json:"slot,omitempty"`        // 插件插槽
	Children    []AdminMenuItem `json:"children,omitempty"`    // 子菜单
}
