package grpcserver

import (
	"context"
	"fmt"
	iamv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/iam/v1"
	"github.com/ArtisanCloud/PowerX/internal/transport/grpc/iam"
	"net"

	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"github.com/ArtisanCloud/PowerX/pkg/utils/logger"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	// 反射 & 健康检查
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

	// Server 选项（TLS 可选）
	var opts []grpc.ServerOption
	if cfg.UseTLS {
		creds, err := credentials.NewServerTLSFromFile(cfg.CertFile, cfg.KeyFile)
		if err != nil {
			return nil, nil, fmt.Errorf("grpc tls: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	} else {
		// 显式声明开发期用明文（可不传）
		_ = insecure.NewCredentials()
	}

	s := grpc.NewServer(opts...)

	// 1) 注册你的业务服务
	iamv1.RegisterMemberServiceServer(s, iam.NewMemberServer(deps))
	iamv1.RegisterTeamServiceServer(s, iam.NewTeamServer(deps))

	// 2) 健康检查（Insomnia/grpcurl 常用）
	if cfg.Health {
		healthpb.RegisterHealthServer(s, health.NewServer())
	}

	// 3) ✅ 反射（Insomnia/Grpcurl/Grpcui 自动列出服务 & 消息）
	ctx := context.Background()
	if cfg.Reflection {
		reflection.Register(s)
		logger.Info(ctx, "[gRPC] server reflection enabled")
	}

	logger.InfoF(ctx, "[gRPC] server built on %s (tls=%v)", addr, cfg.UseTLS)
	return s, lis, nil
}
