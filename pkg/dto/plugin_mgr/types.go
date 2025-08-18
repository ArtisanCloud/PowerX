package plugin_mgr

type PluginMenuItem struct {
	ID    string `json:"id"` // plugin id
	Title string `json:"title"`
	Icon  string `json:"icon"`
	Order int    `json:"order"`
	URL   string `json:"url"` // 宿主可直接打开的 URL（/_p/<id>/admin/…）
}

type PluginItem struct {
	ID       string           `json:"id"`
	Name     string           `json:"name"`
	Version  string           `json:"version"`
	State    string           `json:"state"`     // installed/enabled/disabled
	AdminURL string           `json:"admin_url"` // /_p/<id>/admin/
	APIBase  string           `json:"api_base"`  // /_p/<id>/api
	HasAdmin bool             `json:"has_admin"`
	Menus    []PluginMenuItem `json:"menus"`
}
