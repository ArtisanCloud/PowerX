package bootstrap

import (
	"github.com/ArtisanCloud/PowerX/internal/server/agent"
	"github.com/ArtisanCloud/PowerX/internal/server/agent/handler"
)

// RegisterBuiltinHandlers 把核心 handler 绑定到全局注册表（只做注册，不写实现）
func RegisterBuiltinHandlers() error {
	mgr := agent.GetAgentManager()

	// 统一用一个小工具批量注册，减少重复代码
	items := []struct {
		Use  string
		Func handler.HandlerFunc
		Meta handler.HandlerMeta
	}{
		{Use: "core.debug.seed", Func: handler.CoreDebugPass, Meta: metaCore("seed")},
		{Use: "core.debug.forward", Func: handler.CoreDebugPass, Meta: metaCore("forward")},
		{Use: "core.debug.enrich", Func: handler.CoreDebugPass, Meta: metaCore("enrich")},
		{Use: "core.debug.transform", Func: handler.CoreDebugPass, Meta: metaCore("transform")},
		{Use: "core.debug.emit", Func: handler.CoreDebugPass, Meta: metaCore("emit")},
		{Use: "core.debug.branch", Func: handler.CoreDebugPass, Meta: metaCore("branch")},
		{Use: "core.response.format", Func: handler.CoreResponseFormat, Meta: metaCore("response formatter")},
	}
	for _, it := range items {
		if err := mgr.RegisterHandler(it.Use, it.Func, it.Meta); err != nil {
			return err
		}
	}
	return nil
}

// 小工具
func metaCore(desc string) handler.HandlerMeta {
	return handler.HandlerMeta{
		Scope:       handler.ScopeCore,
		Owner:       "corex",
		Description: desc,
	}
}

// 预留：如果你后面想按租户/应用加载扩展 handler，可在这里追加类似方法：
// func RegisterAppHandlers(tenantID string) error { ... _ = agent.GetAgentManager().RegisterHandler(...) }
