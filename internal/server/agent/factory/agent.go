package factory

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/config"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/contract"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/drivers/eino"
)

// AgentClientCreator 定义创建 Agent 的函数类型
type AgentClientCreator func(config interface{}) (contract.AgentClient, error)

// 注册的驱动创建函数
var registeredDrivers = make(map[string]AgentClientCreator)

// RegisterDriver 注册一个驱动创建函数
func RegisterDriver(name string, creator AgentClientCreator) {
	registeredDrivers[name] = creator
}

// NewAgent 根据配置创建一个 contract.Agent。
func NewAgentClient(ctx context.Context, cfg *config.AgentConfig) (contract.AgentClient, error) {
	driverName := cfg.Driver
	if driverName == "" {
		driverName = config.DefaultDriver
	}

	// 使用反射创建对应的配置结构
	switch driverName {
	case "eino":
		agent, err := eino.NewAgentClient(cfg)
		if err == nil {
			return agent, nil
		}
		return agent, fmt.Errorf("new driver error: %s", err.Error())
	default:
	}
	return nil, fmt.Errorf("unsupported driver: %s", driverName)
}
