package skills

import (
	"context"

	skillsv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/skills/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	skillservice "github.com/ArtisanCloud/PowerX/internal/service/skills"
	skillrepo "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/repository/skills"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type invokeServer struct {
	skillsv1.UnimplementedSkillInvokeServiceServer
	invokeSvc *skillservice.InvokeService
}

func RegisterInvokeService(registrar grpc.ServiceRegistrar, deps *shared.Deps) {
	if registrar == nil || deps == nil || deps.DB == nil {
		return
	}
	registryRepo := skillrepo.NewSkillRegistryRepository(deps.DB)
	traceRepo := skillrepo.NewSkillExecutionTraceRepository(deps.DB)
	auditRepo := skillrepo.NewSkillLifecycleAuditRepository(deps.DB)
	auditSvc := skillservice.NewAuditTraceService(traceRepo, auditRepo)
	skillsv1.RegisterSkillInvokeServiceServer(registrar, &invokeServer{
		invokeSvc: skillservice.NewInvokeService(registryRepo, auditSvc),
	})
}

func (s *invokeServer) InvokeSkill(ctx context.Context, req *skillsv1.InvokeSkillRequest) (*skillsv1.InvokeSkillResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	resolved, err := s.invokeSvc.Resolve(ctx, skillservice.InvokeRequest{
		TenantUUID: req.GetTenantUuid(),
		SkillID:    req.GetSkillId(),
		Version:    req.GetVersion(),
		InvokePath: "grpc.skills.invoke",
		TraceID:    "",
	})
	if err != nil {
		_, envelope := skillservice.MapInvokeError(err)
		return nil, status.Error(codes.InvalidArgument, envelope.Message)
	}
	return &skillsv1.InvokeSkillResponse{
		TraceId:      resolved.TraceID,
		Status:       "completed",
		ProtocolUsed: "skill",
		FallbackUsed: false,
		Result:       req.GetPayload(),
	}, nil
}
