// internal/server/grpc/server.go
package grpcserver

import (
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"net"

	orgv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/iam/v1"
	orggrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/iam"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func New(cfg *GRPCConfig, deps *shared.Deps) (*grpc.Server, net.Listener, error) {
	lis, err := net.Listen("tcp", cfg.Addr())
	if err != nil {
		return nil, nil, err
	}

	s := grpc.NewServer(
	// grpc.ChainUnaryInterceptor(Authz(), Audit(), Trace(), ...),
	)

	// 注册各个 gRPC 服务（适配层：transport/grpc/iam/*）
	orgv1.RegisterMemberServiceServer(s, orggrpc.NewMemberServer(deps))
	orgv1.RegisterTeamServiceServer(s, orggrpc.NewTeamServer(deps))

	// 健康检查 & 反射
	healthpb.RegisterHealthServer(s, health.NewServer())
	reflection.Register(s)

	return s, lis, nil
}
