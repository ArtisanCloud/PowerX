package deps

import (
	"errors"
	"sync"

	"github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/instrumentation"
	manager "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/manager"
	mcpservice "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/mcp"
	tenantservice "github.com/ArtisanCloud/PowerX/internal/service/integration_gateway/tenant"
)

// ToolDependencies 聚合 Integration Gateway MCP 工具运行所需依赖。
type ToolDependencies struct {
	TenantService   *tenantservice.Service
	ManagerService  *manager.Service
	Instrumentation *instrumentation.Instrumentation
}

type ResolvedDependencies struct {
	ToolDependencies
	Adapter *mcpservice.Adapter
}

var (
	mu       sync.RWMutex
	resolved *ResolvedDependencies
)

// Set 更新工具依赖。
func Set(deps ToolDependencies) error {
	if deps.TenantService == nil {
		return errors.New("tenant service is required")
	}
	if deps.ManagerService == nil {
		return errors.New("manager service is required")
	}

	inst := deps.Instrumentation
	if inst == nil {
		inst = instrumentation.NewInstrumentation(nil)
		deps.Instrumentation = inst
	}

	entry := &ResolvedDependencies{
		ToolDependencies: deps,
		Adapter:          mcpservice.NewAdapter(mcpservice.AdapterOptions{Instrumentation: inst}),
	}

	mu.Lock()
	resolved = entry
	mu.Unlock()
	return nil
}

// Get 返回当前依赖配置。
func Get() (*ResolvedDependencies, error) {
	mu.RLock()
	defer mu.RUnlock()
	if resolved == nil {
		return nil, errors.New("integration gateway mcp dependencies not initialized")
	}
	return resolved, nil
}
