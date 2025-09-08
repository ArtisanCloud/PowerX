// internal/transport/grpc/server/grpcserver.go （你那份 New 的改良版）
package grpcserver

import (
	"context"
	"fmt"
	settingv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/setting"
	"net"
	"time"

	agentv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"
	iamv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/iam/v1"
	agentgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/agent"
	"github.com/ArtisanCloud/PowerX/internal/transport/grpc/iam"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	// "google.golang.org/grpc/credentials/insecure" // 服务端明文不需要
	"google.golang.org/grpc/keepalive"

	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func New(cfg *GRPCConfig, deps *shared.Deps) (*grpc.Server, net.Listener, error) {
	port := cfg.Port
	if port == 0 {
		port = 9001
	}
	addr := fmt.Sprintf(":%d", port)

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, nil, err
	}

	var opts []grpc.ServerOption
	if cfg.UseTLS {
		creds, err := credentials.NewServerTLSFromFile(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, nil, fmt.Errorf("grpc tls: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}
	// keepalive（防止中间层断开长流）
	opts = append(opts,
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    2 * time.Minute,  // 空闲多久 ping 一次
			Timeout: 20 * time.Second, // ping 超时
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             1 * time.Minute, // 限制客户端 ping 频率
			PermitWithoutStream: true,            // 无活动流也允许 ping
		}),
	)
	// 调大窗口与消息大小（token/图片/配置较大时更稳）
	opts = append(opts,
		grpc.InitialWindowSize(1<<20),     // 1 MiB
		grpc.InitialConnWindowSize(1<<21), // 2 MiB
		grpc.MaxRecvMsgSize(32<<20),       // 32 MiB
		grpc.MaxSendMsgSize(32<<20),       // 32 MiB
	)

	s := grpc.NewServer(opts...)

	// 业务服务
	iamv1.RegisterMemberServiceServer(s, iam.NewMemberServer(deps))
	iamv1.RegisterTeamServiceServer(s, iam.NewTeamServer(deps))
	agentv1.RegisterAgentStreamServiceServer(s, agentgrpc.NewAgentStreamServer(deps))
	settingv1.RegisterSettingAIServiceServer(s, agentgrpc.NewSettingAIServiceServer(deps))

	// 健康检查
	if cfg.Health {
		healthpb.RegisterHealthServer(s, health.NewServer())
	}

	// 反射
	ctx := context.Background()
	if cfg.Reflection {
		reflection.Register(s)
		logger.Info(ctx, "[gRPC] server reflection enabled")
	}

	logger.InfoF(ctx, "[gRPC] server built on %s (tls=%v)", addr, cfg.UseTLS)
	return s, lis, nil
}
