package plugin_mgr

// dto/plugin_mgr/manifest.go

// 在你已有的 Frontend/Admin 菜单项上补两个字段
type AdminMenu struct {
	Title            string   `yaml:"title" json:"title"`
	Icon             string   `yaml:"icon" json:"icon"`
	Order            int      `yaml:"order" json:"order"`
	Route            string   `yaml:"route,omitempty" json:"route,omitempty"` // 相对插件根的前端路由，默认 "/"
	RequiredPolicies []string `yaml:"required_policies,omitempty" json:"required_policies,omitempty"`
}
