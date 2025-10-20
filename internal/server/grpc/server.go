// internal/server/grpc/server.go
package grpcserver

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	agentv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/agent/v1"
	stsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/auth/sts/v1"
	capabilityRegistryPB "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/registry/v1"
	capv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/capability/v1"
	authorizationpb "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/event_fabric/v1"
	iamv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/iam/v1"
	corexmediav1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/media/v1"
	settingv12 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/setting"
	workflowv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/workflow/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	agentgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/agent"
	authgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/auth"
	middleware2 "github.com/ArtisanCloud/PowerX/internal/transport/grpc/auth/middleware"
	capgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/capability"
	capabilityRegistryGRPC "github.com/ArtisanCloud/PowerX/internal/transport/grpc/capability_registry"
	eventfabricgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/event_fabric"
	"github.com/ArtisanCloud/PowerX/internal/transport/grpc/iam"
	medigrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/media"
	workflowgrpc "github.com/ArtisanCloud/PowerX/internal/transport/grpc/workflow"
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
	unaryInterceptors := []grpc.UnaryServerInterceptor{
		middleware2.UnaryAuth(middleware2.JWTVerifyConfig{Ring: ring}),
	}
	streamInterceptors := []grpc.StreamServerInterceptor{
		middleware2.StreamAuth(middleware2.JWTVerifyConfig{Ring: ring}),
	}
	if deps != nil && deps.EventFabric != nil && deps.EventFabric.Security != nil {
		unaryInterceptors = append(unaryInterceptors, deps.EventFabric.Security.UnaryServerInterceptor())
		streamInterceptors = append(streamInterceptors, deps.EventFabric.Security.StreamServerInterceptor())
	}
	opts = append(opts,
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	)

	// ===== 构建 Server =====
	s := grpc.NewServer(opts...)

	// ===== 注册业务服务 =====
	iamv1.RegisterMemberServiceServer(s, iam.NewMemberServer(deps))
	iamv1.RegisterTeamServiceServer(s, iam.NewTeamServer(deps))

	agentv1.RegisterAgentStreamServiceServer(s, agentgrpc.NewAgentStreamServer(deps))
	settingv12.RegisterSettingAIServiceServer(s, agentgrpc.NewSettingAIServiceServer(deps))
	capv1.RegisterCapabilityRegistryServiceServer(s, capgrpc.NewContractServer(deps))
	if deps.CapabilityRegistrySvc != nil {
		capabilityRegistryGRPC.RegisterCapabilityRegistryServer(s, deps.CapabilityRegistrySvc)
	}
	if deps.RouterSvc != nil {
		capabilityRegistryGRPC.RegisterCapabilityRouterServer(s, deps.RouterSvc)
	}
	if deps.RouterSandboxSvc != nil {
		capabilityRegistryGRPC.RegisterCapabilityRouterSandboxServer(s, deps.RouterSandboxSvc)
	}
	if deps.DiscoverySvc != nil {
		capabilityRegistryGRPC.RegisterCapabilityDiscoveryServer(s, deps.DiscoverySvc)
	}

	// STS（令牌换签/内发）—— 与拦截器共用同一个 KeyRing
	stsv1.RegisterSTSServiceServer(s, authgrpc.NewSTSServiceServerWithRing(deps, ring))

	// Media Asset（复用已有拦截器）
	corexmediav1.RegisterMediaAssetAdminServiceServer(s, medigrpc.NewMediaAssetServer(deps))
	if deps.Workflow != nil && deps.Workflow.Service != nil {
		workflowv1.RegisterWorkflowServiceServer(s, workflowgrpc.NewServer(deps.Workflow.Service))
	}

	ctx := context.Background()

	// ===== 健康检查 =====
	var healthServer *health.Server
	if cfg.Health {
		healthServer = health.NewServer()
		healthpb.RegisterHealthServer(s, healthServer)
		serviceNames := []string{
			iamv1.MemberService_ServiceDesc.ServiceName,
			iamv1.TeamService_ServiceDesc.ServiceName,
			agentv1.AgentStreamService_ServiceDesc.ServiceName,
			settingv12.SettingAIService_ServiceDesc.ServiceName,
			stsv1.STSService_ServiceDesc.ServiceName,
			corexmediav1.MediaAssetAdminService_ServiceDesc.ServiceName,
			capv1.CapabilityRegistryService_ServiceDesc.ServiceName,
			capabilityRegistryPB.CapabilityRegistryService_ServiceDesc.ServiceName,
			capabilityRegistryPB.CapabilityDiscoveryService_ServiceDesc.ServiceName,
			authorizationpb.AuthorizationService_ServiceDesc.ServiceName,
			workflowv1.WorkflowService_ServiceDesc.ServiceName,
		}
		for _, name := range serviceNames {
			healthServer.SetServingStatus(name, healthpb.HealthCheckResponse_SERVING)
		}
	}

	if deps.MediaMgr != nil {
		drivers := strings.Join(deps.MediaMgr.Drivers(), ",")
		if drivers == "" {
			drivers = "<empty>"
		}
		logger.InfoF(ctx, "[gRPC] media manager ready, drivers=%s", drivers)
	}

	if deps.EventFabric != nil {
		eventfabricgrpc.RegisterAuthorizationServer(deps)(s)
	}

	// ===== 反射 =====
	if cfg.Reflection {
		reflection.Register(s)
		logger.Info(ctx, "[gRPC] server reflection enabled")
	}

	logger.InfoF(ctx, "[gRPC] server built on %s (tls=%v)", lis.Addr().String(), cfg.UseTLS)
	return s, lis, nil
}
