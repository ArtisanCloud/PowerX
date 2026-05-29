package manager

import "github.com/ArtisanCloud/PowerX/pkg/plugin_mgr"

// TryInternalToken 尝试获取宿主为该插件注入的内部通信令牌（仅内存）
func TryInternalToken(mgr plugin_mgr.Manager, pluginID string) (string, bool) {
	if impl, ok := mgr.(*managerImpl); ok {
		impl.mu.RLock()
		defer impl.mu.RUnlock()
		t, ok2 := impl.tokens[pluginID]
		return t, ok2
	}
	return "", false
}

func TryRuntimeProcesses(mgr plugin_mgr.Manager, pluginID string) ([]RuntimeProcessView, bool) {
	if impl, ok := mgr.(*managerImpl); ok {
		return impl.RuntimeProcesses(pluginID), true
	}
	return nil, false
}
