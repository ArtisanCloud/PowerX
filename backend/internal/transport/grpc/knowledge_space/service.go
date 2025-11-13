package knowledge_space

import (
	"context"
	"errors"
	"strconv"
	"strings"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/datatypes"
)

// Server implements the KnowledgeSpaceAdminService.
type Server struct {
	knowledgev1.UnimplementedKnowledgeSpaceAdminServiceServer
	svc       *ksvc.Service
	ingestion *ksvc.IngestionService
}

// NewServer builds a gRPC server wrapper.
func NewServer(deps *shared.Deps) *Server {
	if deps == nil || deps.KnowledgeSpace == nil {
		return nil
	}
	return &Server{
		svc:       deps.KnowledgeSpace.Service,
		ingestion: deps.KnowledgeSpace.Ingestion,
	}
}

// Register wires the service into a registrar.
func Register(registrar grpc.ServiceRegistrar, srv *Server) {
	if registrar == nil || srv == nil || srv.svc == nil {
		return
	}
	knowledgev1.RegisterKnowledgeSpaceAdminServiceServer(registrar, srv)
}

func (s *Server) CreateKnowledgeSpace(ctx context.Context, req *knowledgev1.CreateKnowledgeSpaceRequest) (*knowledgev1.CreateKnowledgeSpaceResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(req.GetTenantId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant id: %v", err)
	}
	policyID, err := parsePolicy(req.GetPolicyTemplateVersionId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid policy template id: %v", err)
	}

	space, err := s.svc.CreateSpace(ctx, ksvc.CreateSpaceInput{
		TenantID:       tenantID,
		SpaceName:      req.GetName(),
		DepartmentCode: req.GetDepartmentCode(),
		QuotaCPU:       int(req.GetQuotaCpu()),
		QuotaStorageGB: int(req.GetQuotaStorageGb()),
		PolicyVersion:  policyID,
		FeatureFlags:   req.GetFeatureFlags(),
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &knowledgev1.CreateKnowledgeSpaceResponse{Space: toProto(space)}, nil
}

func (s *Server) UpdateKnowledgeSpace(ctx context.Context, req *knowledgev1.UpdateKnowledgeSpaceRequest) (*knowledgev1.UpdateKnowledgeSpaceResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	policyID, err := parsePolicy(req.GetPolicyTemplateVersionId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid policy template id: %v", err)
	}
	space, err := s.svc.UpdateSpace(ctx, ksvc.UpdateSpaceInput{
		SpaceID:        spaceID,
		QuotaCPU:       int(req.GetQuotaCpu()),
		QuotaStorageGB: int(req.GetQuotaStorageGb()),
		PolicyVersion:  policyID,
		FeatureFlags:   req.GetFeatureFlags(),
		Status:         req.GetStatus(),
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &knowledgev1.UpdateKnowledgeSpaceResponse{Space: toProto(space)}, nil
}

func (s *Server) RetireKnowledgeSpace(ctx context.Context, req *knowledgev1.RetireKnowledgeSpaceRequest) (*knowledgev1.RetireKnowledgeSpaceResponse, error) {
	if s.svc == nil {
		return nil, status.Error(codes.Unavailable, "service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	space, err := s.svc.RetireSpace(ctx, ksvc.RetireSpaceInput{
		SpaceID: spaceID,
		Reason:  req.GetReason(),
	})
	if err != nil {
		return nil, mapError(err)
	}
	return &knowledgev1.RetireKnowledgeSpaceResponse{Space: toProto(space)}, nil
}

func (s *Server) TriggerIngestion(ctx context.Context, req *knowledgev1.IngestionJobRequest) (*knowledgev1.IngestionJobResponse, error) {
	if s.ingestion == nil {
		return nil, status.Error(codes.Unimplemented, "ingestion service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	job, err := s.ingestion.Trigger(ctx, ksvc.TriggerIngestionInput{
		SpaceID:        spaceID,
		SourceType:     req.GetSourceType(),
		SourceURI:      req.GetSourceUri(),
		MaskingProfile: req.GetMaskingProfile(),
		Priority:       req.GetPriority(),
	})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &knowledgev1.IngestionJobResponse{Job: toProtoIngestionJob(job)}, nil
}

func (s *Server) PublishFusionStrategy(ctx context.Context, req *knowledgev1.FusionStrategyRequest) (*knowledgev1.FusionStrategyResponse, error) {
	return nil, status.Error(codes.Unimplemented, "fusion API pending")
}

func (s *Server) SubmitFeedback(ctx context.Context, req *knowledgev1.FeedbackRequest) (*knowledgev1.FeedbackResponse, error) {
	return nil, status.Error(codes.Unimplemented, "feedback API pending")
}

func parsePolicy(v string) (uint64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	return strconv.ParseUint(v, 10, 64)
}

func toProto(space *models.KnowledgeSpace) *knowledgev1.KnowledgeSpace {
	if space == nil {
		return nil
	}
	return &knowledgev1.KnowledgeSpace{
		SpaceId:                 space.UUID.String(),
		TenantId:                space.TenantID.String(),
		Name:                    space.SpaceName,
		DepartmentCode:          space.DepartmentCode,
		Status:                  space.Status,
		QuotaCpu:                uint32(space.QuotaCPU),
		QuotaStorageGb:          uint32(space.QuotaStorageGB),
		FeatureFlags:            decodeFeatureFlags(space.FeatureFlags),
		PolicyTemplateVersionId: strconv.FormatUint(space.PolicyTemplateVersionID, 10),
		CreatedAt:               timestamppb.New(space.CreatedAt),
		UpdatedAt:               timestamppb.New(space.UpdatedAt),
	}
}

func mapError(err error) error {
	switch {
	case ksvc.IsConflictError(err):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, ksvc.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ksvc.ErrSpaceNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ksvc.ErrInvalidStatusTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ksvc.ErrProvisioningBusy):
		return status.Error(codes.Aborted, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func decodeFeatureFlags(raw datatypes.JSON) []string {
	return ksvc.FeatureFlagsFromJSON(raw)
}

func toProtoIngestionJob(job *models.IngestionJob) *knowledgev1.IngestionJobStatus {
	if job == nil {
		return nil
	}
	return &knowledgev1.IngestionJobStatus{
		JobId:               job.UUID.String(),
		Status:              job.Status,
		ChunkTotal:          uint32(job.ChunkTotal),
		ChunkCoveredPct:     float32(job.ChunkCoveredPct),
		EmbeddingSuccessPct: float32(job.EmbeddingSuccessPct),
		MaskingCoveragePct:  float32(job.MaskingCoveragePct),
	}
}
