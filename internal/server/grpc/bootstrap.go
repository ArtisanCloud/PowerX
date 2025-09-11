package grpcserver

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"
	"google.golang.org/grpc"
)

// internal/server/grpc/bootstrap.go

import (
	"context"
	"net"
)

// Bootstrap 负责：推导监听地址 → 构建 gRPC Server → 后台启动 Serve。
// 返回 *grpc.Server（便于在 main 里优雅退出：GracefulStop）。
func BootstrapGRPC(ctx context.Context, cfg *GRPCConfig, deps *shared.Deps) (*grpc.Server, error) {
	// 允许通过配置关闭 gRPC 服务
	if cfg != nil && !cfg.Enable {
		logger.Info(ctx, "[gRPC] disabled by config")
		return nil, nil
	}
	// 构建 Server 与 Listener（复用你在 internal/server/grpc/server.go 提供的 New）
	s, lis, err := New(cfg, deps)

	if err != nil {
		return nil, err
	}

	// 后台启动（非阻塞）
	go ServeGRPC(ctx, s, lis, lis.Addr().String())

	return s, nil
}

// 单独拆一个启动函数，便于测试/复用
func ServeGRPC(ctx context.Context, s *grpc.Server, lis net.Listener, addr string) {
	logger.InfoF(ctx, "[gRPC] listening %s", addr)
	if err := s.Serve(lis); err != nil && err != grpc.ErrServerStopped {
		logger.ErrorF(ctx, "grpc serve error: %v", err)
	}
}
