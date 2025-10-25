package plugin_mgr

type MarketplaceItem struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Author      string   `json:"author"`
	Category    string   `json:"category"`
	Installs    int64    `json:"installs"`
	Icon        string   `json:"icon,omitempty"`
	Tags        []string `json:"tags"`
}
