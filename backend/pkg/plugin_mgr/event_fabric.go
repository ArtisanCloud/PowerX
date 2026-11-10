package plugin_mgr

import "context"

type eventFabricProvisioningKey struct{}

// WithEventFabricProvisioning 仅供已鉴权的安装、版本切换和管理员启用入口使用。
// 此控制位不接受请求体或 manifest 输入；启动恢复直接调用 Enable，不带此标记。
func WithEventFabricProvisioning(ctx context.Context) context.Context {
	return context.WithValue(ctx, eventFabricProvisioningKey{}, true)
}

func EventFabricProvisioning(ctx context.Context) bool {
	allowed, _ := ctx.Value(eventFabricProvisioningKey{}).(bool)
	return allowed
}
