package plugin_mgr

// ------- 安装与运行期公共类型 -------

type InstallOptions struct {
	RunMigrations   bool
	VerifyChecksum  bool
	VerifySignature bool
}

type InstallSource struct {
	LocalFile string
	RemoteURL string
	SHA256    string
	Signature string
}

type MenuOriginType string

const (
	OriginSystem MenuOriginType = "system"
	OriginPlugin MenuOriginType = "plugin"
)

type SlotKey string

const (
	SlotRoot      SlotKey = "group.root"
	SlotPlugins   SlotKey = "group.plugins"
	SlotSettings  SlotKey = "core.settings"
	SlotDashboard SlotKey = "core.dashboard"
	SlotWorkflow  SlotKey = "core.workflow"
	SlotAgent     SlotKey = "core.agent"
	SlotCustom    SlotKey = "group.custom"
)

type MenuKey string

const (
	KeyPlugins   MenuKey = "plugins"
	KeySettings  MenuKey = "settings"
	KeyDashboard MenuKey = "dashboard"
	KeyWorkflow  MenuKey = "workflow"
	KeyAgent     MenuKey = "agent"

	KeyUserManagement MenuKey = "user_management"
	KeyRoleManagement MenuKey = "role_management"
	KeySystemConfig   MenuKey = "system_config"
	KeyAISettings     MenuKey = "ai_settings"
)

type PluginState string

const (
	StateInstalled PluginState = "installed" // 未启用
	StateEnabled   PluginState = "enabled"
	StateDisabled  PluginState = "disabled"
)

// 运行期模型（用于对外返回）
type Plugin struct {
	ID      string      `json:"id"`
	Version string      `json:"version"`
	State   PluginState `json:"state"`

	// 这些来自 manifest：
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Metadata    Metadata `json:"metadata"` // ✅ 建议用值类型，避免 nil

	Runtime     RuntimeSpec      `json:"runtime"`
	Frontend    FrontendSpec     `json:"frontend"`
	Endpoints   EndpointSpec     `json:"endpoints"`
	RBAC        RBACSpec         `json:"rbac"`
	Events      EventSpec        `json:"events"`
	Backend     *BackendSpec     `json:"backend,omitempty"`
	Routes      *RouteSpec       `json:"routes,omitempty"`
	Permissions []PermissionSpec `json:"permissions,omitempty"`
	Menus       []MenuTreeItem   `json:"menus,omitempty"`
	Agents      []AgentSpec      `json:"agents,omitempty"`
	Tools       []ToolSpec       `json:"tools,omitempty"`
	Workflows   []WorkflowSpec   `json:"workflows,omitempty"`

	Paths InstalledPaths `json:"paths"`
}

// ------- 运行形态 -------

type RuntimeKind string

const (
	RuntimeKindProcess RuntimeKind = "process" // 后端子进程
	RuntimeKindStatic  RuntimeKind = "static"  // 仅静态前端
	RuntimeKindRemote  RuntimeKind = "remote"  // 预留：远端服务
)

type HealthCheckSpec struct {
	HTTPPath string `yaml:"http"     json:"http"`     // e.g. "/healthz"
	Interval string `yaml:"interval" json:"interval"` // e.g. "5s"
	Timeout  string `yaml:"timeout"  json:"timeout"`  // e.g. "2s"
}

type RuntimeSpec struct {
	Kind          RuntimeKind       `yaml:"kind"            json:"kind"`
	Entry         string            `yaml:"entry"           json:"entry"` // process 必填
	Args          []string          `yaml:"args"            json:"args"`
	Env           map[string]string `yaml:"env"             json:"env"` // "KEY=VALUE"
	Health        HealthCheckSpec   `yaml:"health"         json:"health"`
	RemoteBaseURL string            `yaml:"remote_base_url" json:"remote_base_url"` // remote 预留
}

// BackendSpec 记录插件后端进程在安装时代码声明的信息
type BackendSpec struct {
	Entry  string `yaml:"entry"  json:"entry"`
	Port   int    `yaml:"port"   json:"port"`
	Health string `yaml:"health" json:"health"`
}

// ------- Endpoints -------

type EndpointSpec struct {
	HTTPBasePath string   `yaml:"http_base_path" json:"http_base_path"` // e.g. "/v1"
	GRPC         GRPCSpec `yaml:"grpc"           json:"grpc"`
}

type GRPCSpec struct {
	Enabled  bool   `yaml:"enabled"   json:"enabled"`
	ProtoDir string `yaml:"proto_dir" json:"proto_dir"`
}

// ------- 前端 -------

type FrontendKind string

const (
	FrontendKindStatic FrontendKind = "static" // 宿主直挂静态资源
	FrontendKindProxy  FrontendKind = "proxy"  // 预留：反代到插件 Admin 服务
)

type FrontendSpec struct {
	Admin FrontendAdminSpec `yaml:"admin" json:"admin"`
}

type FrontendAdminSpec struct {
	Kind          FrontendKind `yaml:"kind"           json:"kind"`       // static | proxy
	StaticDir     string       `yaml:"static_dir"     json:"static_dir"` // kind=static 必填
	ProxyBasePath string       `yaml:"proxy_base_path" json:"proxy_base_path"`
	Menus         []MenuItem   `yaml:"menus"          json:"menus"`
}

type MenuItem struct {
	ID               string     `yaml:"id"    json:"id"`
	Route            string     `yaml:"route" json:"route"` // 相对 admin 根，如 "/", "/reports"
	Path             string     `yaml:"path"  json:"path"`
	Title            string     `yaml:"title" json:"title"`
	Icon             string     `yaml:"icon"  json:"icon"`
	Order            int        `yaml:"order" json:"order"`
	Slot             SlotKey    `yaml:"slot" json:"slot"`
	Visible          *bool      `yaml:"visible" json:"visible"`
	RequiredPolicies []string   `yaml:"required_policies,omitempty" json:"required_policies,omitempty"`
	Children         []MenuItem `yaml:"children,omitempty" json:"children,omitempty"`
}

// ------- RBAC / Events -------

type RBACSpec struct {
	Resources []RBACResource `yaml:"resources" json:"resources"`
}

type PermissionSpec struct {
	Resource string   `yaml:"resource" json:"resource"`
	Actions  []string `yaml:"actions"  json:"actions"`
}

type RBACResource struct {
	Resource string   `yaml:"resource" json:"resource"` // e.g. "hello.report"
	Actions  []string `yaml:"actions"  json:"actions"`  // e.g. ["view","export"]
}

type EventSpec struct {
	Publish   []string `yaml:"publish"   json:"publish"`
	Subscribe []string `yaml:"subscribe" json:"subscribe"`
}

type RouteSpec struct {
	BasePath      string `yaml:"basePath"       json:"basePath"`
	AdminManifest string `yaml:"adminManifest" json:"adminManifest"`
	RBAC          string `yaml:"rbac"          json:"rbac"`
}

type MenuTreeItem struct {
	ID               string         `yaml:"id"       json:"id"`
	Title            string         `yaml:"title"    json:"title"`
	Icon             string         `yaml:"icon"     json:"icon"`
	Path             string         `yaml:"path"     json:"path"`
	Order            int            `yaml:"order"    json:"order"`
	RequiredPolicies []string       `yaml:"required_policies,omitempty" json:"required_policies,omitempty"`
	Children         []MenuTreeItem `yaml:"children,omitempty" json:"children,omitempty"`
}

type AgentSpec struct {
	ID           string   `yaml:"id"           json:"id"`
	PluginID     string   `yaml:"plugin_id"    json:"plugin_id"`
	Name         string   `yaml:"name"         json:"name"`
	Description  string   `yaml:"description"  json:"description"`
	DefaultTools []string `yaml:"default_tools" json:"default_tools"`
}

type JSONSchema map[string]any

type ToolSpec struct {
	ID           string     `yaml:"id"            json:"id"`
	PluginID     string     `yaml:"plugin_id"     json:"plugin_id"`
	Name         string     `yaml:"name"          json:"name"`
	Description  string     `yaml:"description"   json:"description"`
	Transport    string     `yaml:"transport"     json:"transport"`
	Endpoint     string     `yaml:"endpoint"      json:"endpoint"`
	Method       string     `yaml:"method"        json:"method"`
	RBACResource string     `yaml:"rbac_resource" json:"rbac_resource"`
	InputSchema  JSONSchema `yaml:"input_schema"  json:"input_schema"`
	OutputSchema JSONSchema `yaml:"output_schema" json:"output_schema"`
}

type WorkflowSpec struct {
	ID          string `yaml:"id"          json:"id"`
	Name        string `yaml:"name"        json:"name"`
	Description string `yaml:"description" json:"description"`
}

// ------- 安装后的真实落地路径（宿主填充） -------

type InstalledPaths struct {
	Root              string `json:"root"`                // plugins/installed/<id>/<ver>
	FrontendAdminDir  string `json:"frontend_admin_dir"`  // .../frontend/admin
	Entry             string `json:"entry"`               // .../backend/bin/xxx
	PublicDir         string `json:"public_dir"`          // .../public
	MigrationsDir     string `json:"migrations_dir"`      // .../migrations
	ContractsOpenAPI  string `json:"contracts_openapi"`   // .../contracts/openapi.yaml
	ContractsProtoDir string `json:"contracts_proto_dir"` // .../contracts/proto
}
