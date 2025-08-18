package manager

import (
	"fmt"
	"github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"
	"sync"
)

// 用于保存全局 Manager 实例
var (
	global     plugin_mgr.Manager                 // 最终的单例
	once       sync.Once                          // 确保只初始化一次
	provider   func() (plugin_mgr.Manager, error) // 延迟初始化的工厂（由启动阶段注入）
	providerMu sync.RWMutex
	mu         sync.RWMutex
)

// SetGlobalProvider：启动阶段登记“如何创建 Manager”
func SetGlobalProvider(p func() (plugin_mgr.Manager, error)) {
	if p == nil {
		panic("plugin_mgr: SetGlobalProvider called with nil provider")
	}
	providerMu.Lock()
	provider = p
	providerMu.Unlock()
}

// InitGlobal 在应用启动时调用一次（生产环境建议只初始化一次）
func InitGlobal(m plugin_mgr.Manager) {
	if m == nil {
		panic("plugin_mgr: InitGlobal called with nil Manager")
	}
	once.Do(func() {
		mu.Lock()
		global = m
		mu.Unlock()
	})
}

func GetPluginManager() plugin_mgr.Manager {
	// 已经有了就直接返回
	mu.RLock()
	if global != nil {
		g := global
		mu.RUnlock()
		return g
	}
	mu.RUnlock()

	// 第一次：用 provider 懒初始化
	providerMu.RLock()
	p := provider
	providerMu.RUnlock()
	if p == nil {
		panic("plugin_mgr: provider not configured (call SetGlobalProvider in bootstrap)")
	}

	var initErr error
	once.Do(func() {
		m, err := p()
		if err != nil {
			initErr = err
			return
		}
		mu.Lock()
		global = m
		mu.Unlock()
	})
	if initErr != nil {
		panic(fmt.Sprintf("plugin_mgr: provider init failed: %v", initErr))
	}

	// 返回结果
	mu.RLock()
	defer mu.RUnlock()
	return global
}

// ResetGlobalForTest 仅用于测试：允许覆盖全局实例
func ResetGlobalForTest(m plugin_mgr.Manager) {
	mu.Lock()
	defer mu.Unlock()
	global = m
}
