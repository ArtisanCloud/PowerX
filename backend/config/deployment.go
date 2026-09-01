package config

import (
	"fmt"
	"strings"
)

const (
	DeploymentEnvDev     = "dev"
	DeploymentEnvTest    = "test"
	DeploymentEnvStaging = "staging"
	DeploymentEnvProd    = "prod"
)

// DeploymentConfig 描述当前 PowerX Core 实例的稳定部署身份。
type DeploymentConfig struct {
	Env string `yaml:"env" json:"env"`
}

// ValidateDeploymentEnv 严格校验部署环境，不做大小写转换、别名映射或默认推导。
func ValidateDeploymentEnv(raw string) error {
	if raw == "" {
		return fmt.Errorf("deployment.env is required")
	}
	if raw != strings.TrimSpace(raw) {
		return fmt.Errorf("deployment.env must not contain surrounding whitespace")
	}
	switch raw {
	case DeploymentEnvDev, DeploymentEnvTest, DeploymentEnvStaging, DeploymentEnvProd:
		return nil
	default:
		return fmt.Errorf("deployment.env must be one of dev, test, staging, prod")
	}
}

// ValidateDeploymentIdentity 校验当前安装态是否具备合法部署身份。
// 未安装阶段允许暂时为空，以便 /setup 完成首次写入；一旦提供则始终严格校验。
func (c *Config) ValidateDeploymentIdentity() error {
	if c == nil {
		return fmt.Errorf("core config is required")
	}
	if c.Deployment.Env == "" && c.Install.EffectiveStatus() != "installed" {
		return nil
	}
	return ValidateDeploymentEnv(c.Deployment.Env)
}
