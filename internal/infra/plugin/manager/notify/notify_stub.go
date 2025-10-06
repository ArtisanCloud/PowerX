//go:build !plugin_control
// +build !plugin_control

package notify

import (
    "context"
    pmimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
    "github.com/ArtisanCloud/PowerX/pkg/utils/logger"
)

// PushTenantCredentials (stub): 默认构建路径下的空实现，用于避免在未生成 gRPC 代码前的编译错误。
// 生成 proto 并启用真实实现：
//   1) buf generate
//   2) go build -tags plugin_control
func PushTenantCredentials(ctx context.Context, pluginID string, tenantID uint64, clientID, clientSecret string) error {
    // 检查插件运行态与内部令牌是否存在，以便早发现配置问题
    mgr := pmimpl.GetPluginManager()
    if _, ok := pmimpl.TryRuntimeStatus(mgr, pluginID); !ok {
        logger.WarnF(ctx, "[notify] plugin not running: %s", pluginID)
    }
    if tok, ok := pmimpl.TryInternalToken(mgr, pluginID); !ok || tok == "" {
        logger.WarnF(ctx, "[notify] internal token not found for plugin: %s", pluginID)
    }
    // 提示下一步
    logger.InfoF(ctx, "[notify] stub: would push credentials to plugin=%s tenant=%d (enable '-tags plugin_control' after buf generate)", pluginID, tenantID)
    // 将其视为非致命：返回 nil，避免阻断启用/轮换流程
    _ = clientID
    _ = clientSecret
    return nil
}
