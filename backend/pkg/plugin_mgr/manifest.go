package plugin_mgr

// pkg/plugin_mgr/manifest.go

// Manifest 映射 plugin.yaml（声明态）；运行期你会把它 + Paths 组装成 Plugin。
type Manifest struct {
	ID           string `yaml:"id"            json:"id"`
	Version      string `yaml:"version"       json:"version"`
	Name         string `yaml:"name"          json:"name"`
	Description  string `yaml:"description"   json:"description"`
	CoreXVersion string `yaml:"corex_version" json:"corex_version"`

	Runtime      RuntimeSpec        `yaml:"runtime"   json:"runtime"`
	Endpoints    EndpointSpec       `yaml:"endpoints" json:"endpoints"`
	Frontend     FrontendSpec       `yaml:"frontend"  json:"frontend"`
	Catalogs     CatalogSpec        `yaml:"catalogs,omitempty" json:"catalogs,omitempty"`
	Capabilities HostCapabilitySpec `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	Exposure     ExposureSpec       `yaml:"exposure,omitempty" json:"exposure,omitempty"`
	RBAC         RBACSpec           `yaml:"rbac"      json:"rbac"`
	Events       EventSpec          `yaml:"events"    json:"events"`
	Backend      *BackendSpec       `yaml:"backend"   json:"backend,omitempty"`
	Routes       *RouteSpec         `yaml:"routes"    json:"routes,omitempty"`
	Permissions  []PermissionSpec   `yaml:"permissions" json:"permissions,omitempty"`
	Agents       []AgentSpec        `yaml:"agents" json:"agents,omitempty"`
	Tools        []ToolSpec         `yaml:"tools" json:"tools,omitempty"`
	Workflows    []WorkflowSpec     `yaml:"workflows" json:"workflows,omitempty"`

	Migrations *MigrationsSpec `yaml:"migrations" json:"migrations,omitempty"`
	Assets     *AssetsSpec     `yaml:"assets"     json:"assets,omitempty"`
	Checksums  *ChecksumsSpec  `yaml:"checksums"  json:"checksums,omitempty"`
	Signature  *SignatureSpec  `yaml:"signature"  json:"signature,omitempty"`

	Metadata Metadata `yaml:"metadata" json:"metadata"`
}

// HostCapabilitySpec declares the published Core capabilities required by the
// plugin service actor. These are distinct from plugin-provided capabilities.
type HostCapabilitySpec struct {
	Required []string `yaml:"required,omitempty" json:"required,omitempty"`
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
	Driver        string   `yaml:"driver"          json:"driver"`              // "sql"|"goose"|"gorm"|自定义
	Dir           string   `yaml:"dir,omitempty"   json:"dir,omitempty"`       // 兼容旧版：迁移 SQL 所在目录
	Entry         string   `yaml:"entry"           json:"entry"`               // 可执行入口或脚本，相对插件根
	Args          []string `yaml:"args,omitempty"  json:"args,omitempty"`      // 额外参数
	WorkDir       string   `yaml:"workdir,omitempty" json:"workdir,omitempty"` // 执行时工作目录，相对插件根
	Once          bool     `yaml:"once,omitempty"  json:"once,omitempty"`      // true=仅在首次安装执行
	Timeout       string   `yaml:"timeout,omitempty" json:"timeout,omitempty"` // 可选执行超时，格式同 time.ParseDuration
	RollbackEntry string   `yaml:"rollback_entry,omitempty" json:"rollback_entry,omitempty"`
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
