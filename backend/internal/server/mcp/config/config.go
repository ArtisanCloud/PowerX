package config

import (
	"fmt"
)

// MCPConfig MCP服务器配置结构
type MCPConfig struct {
	Server          ServerConfig    `yaml:"server"`
	FlowSpecsConfig FlowSpecsConfig `yaml:"flow_spec"`
	ToolSpecsConfig ToolSpecsConfig `yaml:"tool_spec"`
	TemplatesDir    string          `yaml:"templates_dir"`
	Endpoints       EndpointsConfig `yaml:"endpoints"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	LaunchMode string `yaml:"launch_mode"`
}

// PathsConfig 路径配置
type FlowSpecsConfig struct {
	Blueprints   string `yaml:"blueprints"`
	Capabilities string `yaml:"capabilities"`
	Feedbacks    string `yaml:"feedbacks"`
}

// ToolSpecsConfig 路径配置
type ToolSpecsConfig struct {
	CoreDir string   `yaml:"core_dir"`
	AppDirs []string `yaml:"app_dirs"`
}

// EndpointsConfig 端点配置
type EndpointsConfig struct {
	SSE     string `yaml:"sse"`
	Message string `yaml:"message"`
}

// RegistryConfig 注册表配置
type RegistryConfig struct {
	EnableMetrics    bool     `yaml:"enable_metrics" json:"enable_metrics"`
	EnableVersioning bool     `yaml:"enable_versioning" json:"enable_versioning"`
	DefaultRoles     []string `yaml:"default_roles" json:"default_roles"`
	MaxVersions      int      `yaml:"max_versions" json:"max_versions"`
}

// GetAddress 获取服务器监听地址
func (c *MCPConfig) GetAddress() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}
