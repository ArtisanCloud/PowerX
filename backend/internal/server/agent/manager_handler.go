package agent

import (
	"context"
	"fmt"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/handler"
)

// services/agent/manager_handler.go

// 在 Manager 结构体里建议加上：
//   handlerReg *handler.HandlerRegistry
//   mu         sync.RWMutex
// 并在 NewAgentManager() 里初始化 handlerReg（若之前已做可忽略）

// HandlerRegistry: 读到全局 handler 注册表（只读场景）
func (m *Manager) HandlerRegistry() *handler.HandlerRegistry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.handlerReg
}

// RegisterHandler: 统一入口（标准签名）
func (m *Manager) RegisterHandler(use string, fn handler.HandlerFunc, meta handler.HandlerMeta) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handlerReg == nil {
		m.handlerReg = handler.NewHandlerRegistry()
	}
	return m.handlerReg.Register(use, fn, meta)
}

// RegisterHandlerLegacy: 兼容老签名
func (m *Manager) RegisterHandlerLegacy(
	use string,
	fn func(ctx context.Context, params map[string]any) (map[string]any, error),
	owner, desc string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handlerReg == nil {
		m.handlerReg = handler.NewHandlerRegistry()
	}
	return m.handlerReg.RegisterLegacy(use, fn, owner, desc)
}

// ExecUse: 直接在 Manager 层按 use 执行一次 handler
func (m *Manager) ExecUse(ctx context.Context, use string, ns *handler.Namespace, preferred handler.Scope) (map[string]any, error) {
	m.mu.RLock()
	reg := m.handlerReg
	m.mu.RUnlock()
	if reg == nil {
		return nil, fmt.Errorf("handler registry not initialized")
	}
	return reg.Execute(ctx, use, ns, preferred)
}

/* ==== 新增：可观测/调试用的查询 API ==== */

// ListHandlers: 列出已注册的 handler（可按作用域过滤；0 表示全部）
func (m *Manager) ListHandlers(scope handler.Scope) []handler.Descriptor {
	m.mu.RLock()
	reg := m.handlerReg
	m.mu.RUnlock()
	if reg == nil {
		return nil
	}
	return reg.List(scope)
}

// ListHandlerNames: 仅返回 use 列表（可按作用域过滤；0 表示全部）
func (m *Manager) ListHandlerNames(scope handler.Scope) []string {
	m.mu.RLock()
	reg := m.handlerReg
	m.mu.RUnlock()
	if reg == nil {
		return nil
	}
	return reg.Names(scope)
}

// HasHandler: 判断是否存在某个 use（带首选作用域）
func (m *Manager) HasHandler(use string, preferred handler.Scope) bool {
	m.mu.RLock()
	reg := m.handlerReg
	m.mu.RUnlock()
	if reg == nil {
		return false
	}
	return reg.Has(use, preferred)
}

// GetHandlerMeta: 获取某个 use 的元信息
func (m *Manager) GetHandlerMeta(use string) (handler.HandlerMeta, bool) {
	m.mu.RLock()
	reg := m.handlerReg
	m.mu.RUnlock()
	if reg == nil {
		return handler.HandlerMeta{}, false
	}
	return reg.GetMeta(use)
}

// CountHandlers: 统计数量（可按作用域过滤；0 表示全部）
func (m *Manager) CountHandlers(scope handler.Scope) int {
	m.mu.RLock()
	reg := m.handlerReg
	m.mu.RUnlock()
	if reg == nil {
		return 0
	}
	return reg.Count(scope)
}
