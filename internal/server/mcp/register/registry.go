package register

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/config"
	"github.com/ArtisanCloud/PowerX/internal/server/mcp/types"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/loader"
	"github.com/ArtisanCloud/PowerX/pkg/corex/flow/schemas"
	fmt2 "github.com/ArtisanCloud/PowerX/pkg/utils/fmt"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"sync"
	"time"
)

// mcp/register/registry.go

// ToolEntry 聚合单个工具的所有元数据信息
// 包括规范、处理函数、版本列表、权限列表和指标信息
type ToolEntry struct {
	Spec        *schemas.ToolSpec      // 工具规范
	Handler     server.ToolHandlerFunc // 处理函数
	Versions    []string               // 支持的版本列表
	Permissions []string               // 允许的角色列表
	Metrics     *ToolMetrics           // 使用指标
}

// ToolRegistry 统一管理所有工具的注册表
// 支持并发访问、安全更新及元数据维护
type ToolRegistry struct {
	mu        sync.RWMutex           // 读写锁，保障并发安全
	entries   map[string]*ToolEntry  // key: 工具ID，value: 工具元数据
	createdAt time.Time              // 创建时间
	updatedAt time.Time              // 最近更新时间
	config    *config.RegistryConfig // 注册表配置
}

// ToolMetrics 记录工具的调用统计指标
// 包括调用次数、成功次数、失败次数、上次调用时间及平均耗时
type ToolMetrics struct {
	CallCount    int64     `json:"call_count"`      // 调用总次数
	SuccessCount int64     `json:"success_count"`   // 成功次数
	ErrorCount   int64     `json:"error_count"`     // 失败次数
	LastUsed     time.Time `json:"last_used"`       // 最近使用时间
	AvgDuration  float64   `json:"avg_duration_ms"` // 平均耗时（毫秒）
}

// NewToolMetrics 初始化并返回一个空的指标对象
func NewToolMetrics() *ToolMetrics {
	return &ToolMetrics{}
}

// NewToolRegistry 创建一个新的 ToolRegistry
// 如果 cfg 为空，则使用默认配置
func NewToolRegistry(cfg *config.RegistryConfig) *ToolRegistry {
	if cfg == nil {
		cfg = &config.RegistryConfig{
			EnableMetrics:    true,
			EnableVersioning: true,
			DefaultRoles:     []string{"user"},
			MaxVersions:      5,
		}
	}
	return &ToolRegistry{
		mu:        sync.RWMutex{},
		entries:   make(map[string]*ToolEntry),
		createdAt: time.Now(),
		updatedAt: time.Now(),
		config:    cfg,
	}
}

// RegisterToolWithSpec 注册一个带规范的工具及其处理函数
// 支持版本校验、权限初始化和指标初始化
func (r *ToolRegistry) RegisterToolWithSpec(spec *schemas.ToolSpec, handler server.ToolHandlerFunc) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 校验规范合法性
	if err := r.validateToolSpec(spec); err != nil {
		return fmt.Errorf("无效的工具规范: %w", err)
	}

	// 获取或创建 ToolEntry
	entry, exists := r.entries[spec.ID]
	if !exists {
		entry = &ToolEntry{
			Spec:        spec,
			Handler:     handler,
			Versions:    []string{spec.Version},
			Permissions: spec.AllowedRoles,
			Metrics:     NewToolMetrics(),
		}
		r.entries[spec.ID] = entry
	} else {
		// 已存在则更新
		entry.Spec = spec
		// 版本冲突检查
		for _, v := range entry.Versions {
			if v == spec.Version {
				return fmt.Errorf("工具 %s 的版本 %s 已存在", spec.ID, spec.Version)
			}
		}
		entry.Versions = append(entry.Versions, spec.Version)
		entry.Handler = handler
		if len(spec.AllowedRoles) > 0 {
			entry.Permissions = spec.AllowedRoles
		}
	}

	r.updatedAt = time.Now()
	return nil
}

// RegisterToolHandler 注册或更新工具的处理函数（兼容旧接口）
func (r *ToolRegistry) RegisterToolHandler(toolID string, handler server.ToolHandlerFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.entries[toolID]
	if !exists {
		entry = &ToolEntry{
			Handler:     handler,
			Permissions: r.config.DefaultRoles,
			Metrics:     NewToolMetrics(),
		}
		r.entries[toolID] = entry
	} else {
		entry.Handler = handler
	}
	r.updatedAt = time.Now()
}

// registerToolSpecOnly 仅注册工具规范，不含处理函数
func (r *ToolRegistry) registerToolSpecOnly(toolID string, spec *schemas.ToolSpec) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.entries[toolID]
	if !exists {
		entry = &ToolEntry{
			Spec:        spec,
			Versions:    []string{spec.Version},
			Permissions: spec.AllowedRoles,
			Metrics:     NewToolMetrics(),
		}
		r.entries[toolID] = entry
	} else {
		entry.Spec = spec
		// 保证版本唯一
		found := false
		for _, v := range entry.Versions {
			if v == spec.Version {
				found = true
			}
		}
		if !found {
			entry.Versions = append(entry.Versions, spec.Version)
		}
		if len(spec.AllowedRoles) > 0 {
			entry.Permissions = spec.AllowedRoles
		}
	}
	r.updatedAt = time.Now()
}

// GetToolSpec 返回指定工具的规范，如不存在返回 false
func (r *ToolRegistry) GetToolSpec(toolID string) (*schemas.ToolSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.entries[toolID]
	if !exists || entry.Spec == nil {
		return nil, false
	}
	return entry.Spec, true
}

// GetToolHandler 返回指定工具的处理函数，如不存在返回 false
func (r *ToolRegistry) GetToolHandler(toolID string) (server.ToolHandlerFunc, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.entries[toolID]
	if !exists || entry.Handler == nil {
		return nil, false
	}
	return entry.Handler, true
}

// GetAllHandlers 列出所有已注册的处理函数
func (r *ToolRegistry) GetAllHandlers() map[string]server.ToolHandlerFunc {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]server.ToolHandlerFunc)
	for id, entry := range r.entries {
		if entry.Handler != nil {
			out[id] = entry.Handler
		}
	}
	return out
}

// GetAllEntries 返回所有的 ToolEntry（副本），包含 Spec、Handler、Metrics 等
func (r *ToolRegistry) GetAllEntries() map[string]ToolEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]ToolEntry, len(r.entries))
	for id, entry := range r.entries {
		// 注意：这里是浅拷贝，如果需要防止外部修改，可再深拷贝字段
		out[id] = *entry
	}
	return out
}

// GetAllToolSpecsTyped 列出所有已注册的规范（类型安全）
func (r *ToolRegistry) GetAllToolSpecsTyped() map[string]*schemas.ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]*schemas.ToolSpec)
	for id, entry := range r.entries {
		if entry.Spec != nil {
			out[id] = entry.Spec
		}
	}
	return out
}

// CheckPermission 校验用户角色是否有权限调用工具
func (r *ToolRegistry) CheckPermission(toolID, userRole string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.entries[toolID]
	if !exists || len(entry.Permissions) == 0 {
		return true
	}
	for _, role := range entry.Permissions {
		if role == userRole || role == "*" {
			return true
		}
	}
	return false
}

// RecordMetrics 记录工具使用指标（调用次数、成功/失败次数、耗时等）
func (r *ToolRegistry) RecordMetrics(toolID string, success bool, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.entries[toolID]
	if !exists {
		entry = &ToolEntry{Metrics: NewToolMetrics()}
		r.entries[toolID] = entry
	}

	m := entry.Metrics
	m.CallCount++
	m.LastUsed = time.Now()
	if success {
		m.SuccessCount++
	} else {
		m.ErrorCount++
	}
	// 更新平均耗时
	ms := float64(duration.Nanoseconds()) / 1e6
	if m.AvgDuration == 0 {
		m.AvgDuration = ms
	} else {
		m.AvgDuration = (m.AvgDuration + ms) / 2
	}
}

// GetMetrics 获取单个工具的指标
func (r *ToolRegistry) GetMetrics(toolID string) (*ToolMetrics, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.entries[toolID]
	if !exists || entry.Metrics == nil {
		return nil, false
	}
	return entry.Metrics, true
}

// GetAllMetrics 获取所有工具的指标
func (r *ToolRegistry) GetAllMetrics() map[string]*ToolMetrics {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(map[string]*ToolMetrics)
	for id, entry := range r.entries {
		out[id] = entry.Metrics
	}
	return out
}

// GetVersions 列出指定工具的所有版本
func (r *ToolRegistry) GetVersions(toolID string) ([]string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.entries[toolID]
	if !exists {
		return nil, false
	}
	return entry.Versions, true
}

// UnregisterTool 注销工具，移除所有相关数据
func (r *ToolRegistry) UnregisterTool(toolID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.entries[toolID]; !exists {
		return false
	}
	delete(r.entries, toolID)
	r.updatedAt = time.Now()
	return true
}

// validateToolSpec 验证 ToolSpec 必填字段
func (r *ToolRegistry) validateToolSpec(spec *schemas.ToolSpec) error {
	if spec.ID == "" {
		return fmt.Errorf("tool ID 是必须项")
	}
	if spec.Name == "" {
		return fmt.Errorf("tool Name 是必须项")
	}
	if spec.Version == "" {
		return fmt.Errorf("tool Version 是必须项")
	}
	if spec.InputSchema == nil {
		return fmt.Errorf("tool InputSchema 是必须项")
	}
	return nil
}

// GetToolDescription 获取工具描述，优先来自规范，其次预定义
func (r *ToolRegistry) GetToolDescription(toolID string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if entry, exists := r.entries[toolID]; exists && entry.Spec != nil {
		return entry.Spec.Description
	}
	// 使用预定义的描述，如果有的话
	fallback := map[string]string{
		types.ToolLoadBlueprint:  "加载蓝图文件",
		types.ToolPlanFlow:       "规划执行流程",
		types.ToolRenderPlan:     "渲染执行计划为代码",
		types.ToolListBlueprints: "列出所有蓝图文件",
	}
	if d, ok := fallback[toolID]; ok {
		return d
	}
	return fmt.Sprintf("CoreX %s 工具", toolID)
}

// GetRegistryInfo 获取注册表概要信息
func (r *ToolRegistry) GetRegistryInfo() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return map[string]interface{}{
		"tool_count":  len(r.entries),
		"created_at":  r.createdAt,
		"updated_at":  r.updatedAt,
		"total_calls": r.getTotalCalls(),
	}
}

// getTotalCalls 统计所有工具的调用总次数
func (r *ToolRegistry) getTotalCalls() int64 {
	var total int64
	for _, entry := range r.entries {
		total += entry.Metrics.CallCount
	}
	return total
}

// 全局单例：统一工具注册表
var globalRegistry *ToolRegistry

// GetGlobalRegistry 获取全局工具注册表实例
func GetGlobalRegistry() *ToolRegistry {
	if globalRegistry == nil {
		globalRegistry = NewToolRegistry(nil)
	}
	return globalRegistry
}

// LoadToolSpecsFromConfig 从配置加载并注册 ToolSpec
func LoadToolSpecsFromConfig(cfg *config.MCPConfig) error {
	specLoader := loader.NewYAMLSpecLoader("")

	// 确保全局注册表已初始化
	registry := GetGlobalRegistry()

	// 核心工具规范
	if cfg.ToolSpecsConfig.CoreDir != "" {
		specs, err := specLoader.LoadToolSpecsFromDir(cfg.ToolSpecsConfig.CoreDir)
		if err != nil {
			return fmt.Errorf("加载核心工具规范失败: %w", err)
		}
		// fmt2.Dump("check tools specs", specs)
		for id, spec := range specs {
			registry.registerToolSpecOnly(id, spec)
		}
	}
	// 应用层工具规范
	for _, dir := range cfg.ToolSpecsConfig.AppDirs {
		if dir == "" {
			continue
		}
		specs, err := specLoader.LoadToolSpecsFromDir(dir)
		fmt2.Dump("check app specs", specs)
		if err != nil {
			fmt.Printf("Warning: 从 %s 加载应用规范失败: %v\n", dir, err)
			continue
		}
		for id, spec := range specs {
			registry.registerToolSpecOnly(id, spec)
		}
	}

	return nil
}

// RegisterTools 将注册表中的工具挂载到 MCPServer
func RegisterToolsToServer(srv *server.MCPServer) {
	// 已有 Handler
	for id, handler := range globalRegistry.GetAllHandlers() {
		tool := mcp.NewTool(id)
		tool.Description = globalRegistry.GetToolDescription(id)
		srv.AddTool(tool, handler)
	}
	// 动态生成 Handler
	for id, spec := range globalRegistry.GetAllToolSpecsTyped() {
		entry, _ := globalRegistry.entries[id]
		if entry.Handler != nil {
			continue
		}
		h := createHandlerFromSpec(spec)
		if h != nil {
			globalRegistry.RegisterToolHandler(id, h)
			tool := mcp.NewTool(id)
			tool.Description = spec.Description
			srv.AddTool(tool, h)
		}
	}
}

// createHandlerFromSpec 基于 ToolSpec 创建动态 Handler
func createHandlerFromSpec(spec *schemas.ToolSpec) server.ToolHandlerFunc {
	return GetToolSpecHandler().CreateDynamicHandler(spec)
}

// RegisterAppTool 允许应用层动态注册自定义工具
func RegisterAppTool(id string, handler server.ToolHandlerFunc) {
	globalRegistry.RegisterToolHandler(id, handler)
}
