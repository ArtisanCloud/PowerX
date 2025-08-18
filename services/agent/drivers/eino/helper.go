package eino

import "strings"

// 设置/追加 Flow 别名
func (a *Agent) SetFlowAliases(m map[string]string) {
	a.flowMu.Lock()
	defer a.flowMu.Unlock()
	if a.flowAliases == nil {
		a.flowAliases = map[string]string{}
	}
	for k, v := range m {
		kk := strings.TrimSpace(k)
		vv := strings.TrimSpace(v)
		if kk != "" && vv != "" {
			a.flowAliases[kk] = vv
		}
	}
}

// 读取别名（只做简单映射；更激进的启发式放在 getOrBuildFlow 里处理）
func (a *Agent) aliasOf(id string) (string, bool) {
	a.flowMu.RLock()
	defer a.flowMu.RUnlock()
	v, ok := a.flowAliases[id]
	return v, ok
}
