package eino

import "strings"

// 设置/追加 Flow 别名
func (a *AgentClient) aliasOf(id string) (string, bool) {
	a.flowMu.RLock()
	defer a.flowMu.RUnlock()
	v, ok := a.flowAliases[id]
	return v, ok && strings.TrimSpace(v) != ""
}

func (a *AgentClient) SetFlowAliases(m map[string]string) {
	a.flowMu.Lock()
	defer a.flowMu.Unlock()
	for k, v := range m {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" && v != "" {
			a.flowAliases[k] = v
		}
	}
}
