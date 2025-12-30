//go:build plugin_control
// +build plugin_control

package notify

import (
	"context"
	"fmt"
	"time"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	pcv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/plugin/control/v1"
	pmimpl "github.com/ArtisanCloud/PowerX/internal/infra/plugin/manager"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// PushTenantCredentials: 宿主→插件 gRPC 下发凭证（best-effort）
func PushTenantCredentials(ctx context.Context, pluginID, tenantUUID, clientID, clientSecret string) error {
	mgr := pmimpl.GetPluginManager()
	// 取运行端口
	info, ok := pmimpl.TryRuntimeStatus(mgr, pluginID)
	if !ok || info.Port == 0 {
		return fmt.Errorf("plugin not running")
	}
	// 取内部 token
	tok, ok := pmimpl.TryInternalToken(mgr, pluginID)
	if !ok || tok == "" {
		return fmt.Errorf("internal token not found")
	}
	addr := fmt.Sprintf("127.0.0.1:%d", info.Port)
	// 明文 gRPC（本机回环口）
	tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cc, err := grpc.DialContext(tctx, addr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return err
	}
	defer cc.Close()
	cli := pcv1.NewControlServiceClient(cc)
	md := metadata.Pairs("authorization", "Bearer "+tok)
	ctx = metadata.NewOutgoingContext(ctx, md)
	_, err = cli.UpsertTenantCredentials(ctx, &pcv1.UpsertTenantCredentialsRequest{
		Ctx:          &commonv1.RequestContext{TenantUuid: tenantUUID},
		TenantUuid:   tenantUUID,
		PluginId:     pluginID,
		ClientId:     clientID,
		ClientSecret: clientSecret,
	})
	if err != nil {
		logger.WarnF(ctx, "push credentials failed: plugin=%s tenant=%s err=%v", pluginID, tenantUUID, err)
		return err
	}
	return nil
}
