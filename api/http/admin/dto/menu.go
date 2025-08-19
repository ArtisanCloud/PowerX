package dto

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
	Children    []AdminMenuItem `json:"children,omitempty"`
}
