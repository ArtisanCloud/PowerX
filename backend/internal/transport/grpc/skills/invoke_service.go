package skills

import (
	"context"
	"fmt"

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
	payload := make(map[string]interface{}, len(req.GetPayload()))
	for k, v := range req.GetPayload() {
		payload[k] = v
	}
	executed, err := s.invokeSvc.Execute(ctx, skillservice.InvokeRequest{
		TenantUUID: req.GetTenantUuid(),
		SkillID:    req.GetSkillId(),
		Version:    req.GetVersion(),
		InvokePath: "grpc.skills.invoke",
		TraceID:    "",
	}, payload, nil)
	if err != nil {
		_, envelope := skillservice.MapInvokeError(err)
		return nil, status.Error(codes.InvalidArgument, envelope.Message)
	}
	result := make(map[string]string, len(executed.Result))
	for k, v := range executed.Result {
		result[k] = fmt.Sprintf("%v", v)
	}
	return &skillsv1.InvokeSkillResponse{
		TraceId:      executed.TraceID,
		Status:       executed.Status,
		ProtocolUsed: executed.ProtocolUsed,
		FallbackUsed: executed.FallbackUsed,
		Result:       result,
	}, nil
}
