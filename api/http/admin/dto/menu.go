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
	URL         string          `json:"path,omitempty"`
	Order       int             `json:"order"`
	Visible     bool            `json:"visible"`
	Origin      string          `json:"origin,omitempty"`
	Permissions []string        `json:"permissions,omitempty"`
	ParentID    string          `json:"parentId,omitempty"`
	Slot        string          `json:"slot,omitempty"` // 插槽
	Children    []AdminMenuItem `json:"children,omitempty"`
}
