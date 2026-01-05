package plugin_mgr

import "time"

// ------- 安装与运行期公共类型 -------

type InstallOptions struct {
	RunMigrations   bool
	VerifyChecksum  bool
	VerifySignature bool
	Force           bool
	AutoEnable      bool
	HostConfigSeed  *HostConfig
	Metadata        InstallMetadata
}

// InstallMetadata 记录一次安装请求携带的额外上下文。
type InstallMetadata struct {
	Scope       string             `json:"scope"`
	Namespace   string             `json:"namespace"`
	Environment string             `json:"environment"`
	AutoUpdate  bool               `json:"auto_update"`
	Permissions InstallPermissions `json:"permissions"`
	Notes       string             `json:"notes"`
}

// InstallPermissions 描述插件在安装阶段声明的最小权限需求。
type InstallPermissions struct {
	Network bool `json:"network"`
	Storage bool `json:"storage"`
	Files   bool `json:"files"`
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
	KeyPlugins        MenuKey = "plugins"
	KeySettings       MenuKey = "settings"
	KeyDashboard      MenuKey = "dashboard"
	KeyWorkflow       MenuKey = "workflow"
	KeyMedia          MenuKey = "media"
	KeyAgent          MenuKey = "agent"
	KeyKnowledgeSpace MenuKey = "knowledge_space"

	KeyUserManagement     MenuKey = "user_management"
	KeyRoleManagement     MenuKey = "role_management"
	KeySystemConfig       MenuKey = "system_config"
	KeyAISettings         MenuKey = "ai_settings"
	KeyAISettingsModel    MenuKey = "ai_settings_model"
	KeyAISettingsCost     MenuKey = "ai_settings_cost"
	KeyAISettingsRegistry MenuKey = "ai_settings_registry"
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
	Agents      []AgentSpec      `json:"agents,omitempty"`
	Tools       []ToolSpec       `json:"tools,omitempty"`
	Workflows   []WorkflowSpec   `json:"workflows,omitempty"`

	Paths InstalledPaths `json:"paths"`

	HostConfig      *HostConfig      `json:"host_config,omitempty"`
	Migration       *MigrationRecord `json:"migration,omitempty"`
	InstallMetadata InstallMetadata  `json:"install_metadata,omitempty"`
}

type MigrationStatus string

const (
	MigrationStatusUnknown MigrationStatus = ""
	MigrationStatusSuccess MigrationStatus = "success"
	MigrationStatusFailed  MigrationStatus = "failed"
	MigrationStatusSkipped MigrationStatus = "skipped"
)

type MigrationRecord struct {
	Entry      string          `json:"entry"`
	Args       []string        `json:"args,omitempty"`
	WorkDir    string          `json:"workdir,omitempty"`
	Once       bool            `json:"once,omitempty"`
	Timeout    string          `json:"timeout,omitempty"`
	Hash       string          `json:"hash,omitempty"`
	LastStatus MigrationStatus `json:"last_status,omitempty"`
	ExecutedAt *time.Time      `json:"executed_at,omitempty"`
	LastError  string          `json:"last_error,omitempty"`
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
	FrontendKindStatic  FrontendKind = "static"  // 宿主直挂静态资源
	FrontendKindProxy   FrontendKind = "proxy"   // 宿主反代到插件已有 Admin 服务（同后端或远端）
	FrontendKindProcess FrontendKind = "process" // 宿主启动一个独立的 Admin 进程（如 Nuxt/Nitro）
)

type FrontendSpec struct {
	Admin FrontendAdminSpec `yaml:"admin" json:"admin"`
}

type FrontendAdminSpec struct {
	Kind          FrontendKind `yaml:"kind"            json:"kind"`                // static | proxy | process
	StaticDir     string       `yaml:"static_dir"      json:"static_dir"`          // kind=static 必填
	ProxyBasePath string       `yaml:"proxy_base_path" json:"proxy_base_path"`     // kind=proxy 可选（与后端共端口时使用）
	Process       *RuntimeSpec `yaml:"process,omitempty" json:"process,omitempty"` // kind=process 必填（描述 Nuxt/Nitro 进程）
	Menus         []MenuItem   `yaml:"menus"           json:"menus"`
	I18n          *I18nSpec    `yaml:"i18n,omitempty"  json:"i18n,omitempty"`
}

type MenuItem struct {
	ID               string     `yaml:"id"    json:"id"`
	Route            string     `yaml:"route" json:"route"` // 相对 admin 根，如 "/", "/reports"
	Path             string     `yaml:"path"  json:"path"`
	Title            string     `yaml:"title" json:"title"`
	TitleI18n        *MenuLabel `yaml:"title_i18n,omitempty" json:"title_i18n,omitempty"`
	Icon             string     `yaml:"icon"  json:"icon"`
	Order            int        `yaml:"order" json:"order"`
	Slot             SlotKey    `yaml:"slot" json:"slot"`
	Visible          *bool      `yaml:"visible" json:"visible"`
	RequiredPolicies []string   `yaml:"required_policies,omitempty" json:"required_policies,omitempty"`
	Children         []MenuItem `yaml:"children,omitempty" json:"children,omitempty"`
}

type MenuLabel struct {
	Namespace string `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	Key       string `yaml:"key" json:"key"`
	Default   string `yaml:"default,omitempty" json:"default,omitempty"`
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

type I18nSpec struct {
	Dir              string   `yaml:"dir" json:"dir"`
	Format           string   `yaml:"format,omitempty" json:"format,omitempty"`
	DefaultNamespace string   `yaml:"default_namespace,omitempty" json:"default_namespace,omitempty"`
	Namespaces       []string `yaml:"namespaces,omitempty" json:"namespaces,omitempty"`
	Locales          []string `yaml:"locales,omitempty" json:"locales,omitempty"`
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

// HostConfig 记录宿主为插件生成的环境变量映射及落盘位置
type HostConfig struct {
	ValuesFile  string            `json:"values_file"`
	Values      map[string]string `json:"values"`
	GeneratedAt time.Time         `json:"generated_at"`
	Spec        map[string]any    `json:"spec,omitempty"`
}

// ------- 安装后的真实落地路径（宿主填充） -------

type InstalledPaths struct {
	Root                 string `json:"root"`               // plugins/installed/<id>/<ver>
	FrontendAdminDir     string `json:"frontend_admin_dir"` // .../frontend/admin
	FrontendAdminI18nDir string `json:"frontend_admin_i18n_dir,omitempty"`
	Entry                string `json:"entry"`          // .../backend/bin/xxx
	PublicDir            string `json:"public_dir"`     // .../public
	MigrationsDir        string `json:"migrations_dir"` // .../migrations
	MigrationsEntry      string `json:"migrations_entry"`
	MigrationsWorkDir    string `json:"migrations_workdir"`
	ContractsOpenAPI     string `json:"contracts_openapi"`   // .../contracts/openapi.yaml
	ContractsProtoDir    string `json:"contracts_proto_dir"` // .../contracts/proto
	ConfigDir            string `json:"config_dir"`
	HostValuesFile       string `json:"host_values_file"`
}
