// internal/server/grpc/server.go
package grpcserver

import (
	"context"
	"fmt"
	settingv12 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/setting"
	middleware2 "github.com/ArtisanCloud/PowerX/internal/transport/grpc/auth/middleware"
	"net"
	"time"

	agentv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"
	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"
	iamv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/iam/v1"
	corexmediav1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/media/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/agent"
	authgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/auth"
	"github.com/ArtisanCloud/PowerX/internal/transport/grpc/iam"
	medigrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/media"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	// 服务端明文无需 insecure
	"google.golang.org/grpc/keepalive"

	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func New(cfg *GRPCConfig, deps *shared.Deps) (*grpc.Server, net.Listener, error) {
	// 监听地址与网络协议从配置推导
	addr := cfg.Addr()
	network := cfg.Network
	if network == "" {
		network = "tcp"
	}

	lis, err := net.Listen(network, addr)
	if err != nil {
		return nil, nil, err
	}

	var opts []grpc.ServerOption

	// ===== TLS（可选）=====
	if cfg.UseTLS {
		creds, err := credentials.NewServerTLSFromFile(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, nil, fmt.Errorf("grpc tls: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	// ===== Keepalive（长流更稳，防中间层断开）=====
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

	// ===== 窗口 & 消息大小（更宽松，适配流/图片/配置）=====
	opts = append(opts,
		grpc.InitialWindowSize(1<<20),     // 1 MiB
		grpc.InitialConnWindowSize(1<<21), // 2 MiB
		grpc.MaxRecvMsgSize(32<<20),       // 32 MiB
		grpc.MaxSendMsgSize(32<<20),       // 32 MiB
	)

	// ===== 鉴权拦截器（kid + 多密钥兜底；与 STS 共用 KeyRing）=====
	ring := authgrpc.NewDefaultKeyRing(deps)
	opts = append(opts,
		grpc.ChainUnaryInterceptor(
			middleware2.UnaryAuth(middleware2.JWTVerifyConfig{Ring: ring}),
		),
		grpc.ChainStreamInterceptor(
			middleware2.StreamAuth(middleware2.JWTVerifyConfig{Ring: ring}),
		),
	)

	// ===== 构建 Server =====
	s := grpc.NewServer(opts...)

	// ===== 注册业务服务 =====
	iamv1.RegisterMemberServiceServer(s, iam.NewMemberServer(deps))
	iamv1.RegisterTeamServiceServer(s, iam.NewTeamServer(deps))

	agentv1.RegisterAgentStreamServiceServer(s, agentgrpc.NewAgentStreamServer(deps))
	settingv12.RegisterSettingAIServiceServer(s, agentgrpc.NewSettingAIServiceServer(deps))

	// STS（令牌换签/内发）—— 与拦截器共用同一个 KeyRing
	stsv1.RegisterSTSServiceServer(s, authgrpc.NewSTSServiceServerWithRing(deps, ring))

	// Media Asset
	corexmediav1.RegisterMediaAssetAdminServiceServer(s, medigrpc.NewMediaAssetServer(deps))

	// ===== 健康检查 =====
	if cfg.Health {
		healthpb.RegisterHealthServer(s, health.NewServer())
	}

	// ===== 反射 =====
	ctx := context.Background()
	if cfg.Reflection {
		reflection.Register(s)
		logger.Info(ctx, "[gRPC] server reflection enabled")
	}

	logger.InfoF(ctx, "[gRPC] server built on %s (tls=%v)", lis.Addr().String(), cfg.UseTLS)
	return s, lis, nil
}
