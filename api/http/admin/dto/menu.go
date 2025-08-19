package dto

type AdminMenuItem struct {
	Key         string          `json:"key"` // 唯一键（建议 system:xxx / plugin:<id>:<route>）
	Title       string          `json:"title"`
	Icon        string          `json:"icon,omitempty"`
	URL         string          `json:"url,omitempty"`
	Order       int             `json:"order"`
	Children    []AdminMenuItem `json:"children,omitempty"`
	Origin      string          `json:"origin"`                // "system" | "plugin"
	Permissions []string        `json:"permissions,omitempty"` // ["resource:action"]
}
