package plugin_mgr

// pkg/plugin_mgr/manifest.go

// Manifest 映射 plugin.yaml（声明态）；运行期你会把它 + Paths 组装成 Plugin。
type Manifest struct {
	ID           string `yaml:"id"            json:"id"`
	Version      string `yaml:"version"       json:"version"`
	Name         string `yaml:"name"          json:"name"`
	Description  string `yaml:"description"   json:"description"`
	CoreXVersion string `yaml:"corex_version" json:"corex_version"`

	Runtime     RuntimeSpec      `yaml:"runtime"   json:"runtime"`
	Endpoints   EndpointSpec     `yaml:"endpoints" json:"endpoints"`
	Frontend    FrontendSpec     `yaml:"frontend"  json:"frontend"`
	RBAC        RBACSpec         `yaml:"rbac"      json:"rbac"`
	Events      EventSpec        `yaml:"events"    json:"events"`
	Backend     *BackendSpec     `yaml:"backend"   json:"backend,omitempty"`
	Routes      *RouteSpec       `yaml:"routes"    json:"routes,omitempty"`
	Permissions []PermissionSpec `yaml:"permissions" json:"permissions,omitempty"`
	Menus       []MenuTreeItem   `yaml:"menus" json:"menus,omitempty"`
	Agents      []AgentSpec      `yaml:"agents" json:"agents,omitempty"`
	Tools       []ToolSpec       `yaml:"tools" json:"tools,omitempty"`
	Workflows   []WorkflowSpec   `yaml:"workflows" json:"workflows,omitempty"`

	Migrations *MigrationsSpec `yaml:"migrations" json:"migrations,omitempty"`
	Assets     *AssetsSpec     `yaml:"assets"     json:"assets,omitempty"`
	Checksums  *ChecksumsSpec  `yaml:"checksums"  json:"checksums,omitempty"`
	Signature  *SignatureSpec  `yaml:"signature"  json:"signature,omitempty"`

	Metadata Metadata `yaml:"metadata" json:"metadata"`
}

type Metadata struct {
	Author   string   `yaml:"author"   json:"author"`
	Category string   `yaml:"category" json:"category"`
	Tags     []string `yaml:"tags"     json:"tags"`
	Icon     string   `yaml:"icon"     json:"icon"`
	Homepage string   `yaml:"homepage" json:"homepage"`
	License  string   `yaml:"license"  json:"license"`
}

type MigrationsSpec struct {
	Driver string `yaml:"driver" json:"driver"` // "sql"|"goose"|"gorm"
	Dir    string `yaml:"dir"    json:"dir"`    // 相对插件根，如 "./migrations"
}

type AssetsSpec struct {
	PublicDir    string `yaml:"public_dir" json:"public_dir"` // "./public"
	WebAdminPath string `yaml:"webAdminPath" json:"webAdminPath"`
}

type ChecksumsSpec struct {
	PackageSHA256 string `yaml:"package_sha256" json:"package_sha256"`
}

type SignatureSpec struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	// 预留：issuer/alg 等
}
