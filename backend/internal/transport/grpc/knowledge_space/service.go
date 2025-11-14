package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/context_snapshot"
	ksdelta "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/delta"
	qaBridge "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/qa_bridge"
	"github.com/ArtisanCloud/PowerX/internal/service/knowledge_space/toolchain"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gorm.io/datatypes"
)

// Server implements KnowledgeSpaceAdminService + QABridge APIs.
type Server struct {
	knowledgev1.UnimplementedKnowledgeSpaceAdminServiceServer
	knowledgev1.UnimplementedKnowledgeSpaceQABridgeServiceServer
	svc       *ksvc.Service
	ingestion *ksvc.IngestionService
	fusion    *ksvc.FusionService
	feedback  *ksvc.FeedbackService
	delta     *ksdelta.Service
	qa        *qaBridge.Service
}

// NewServer builds a gRPC server wrapper.
func NewServer(deps *shared.Deps) *Server {
	if deps == nil || deps.KnowledgeSpace == nil {
		return nil
	}
	return &Server{
		svc:       deps.KnowledgeSpace.Service,
		ingestion: deps.KnowledgeSpace.Ingestion,
		fusion:    deps.KnowledgeSpace.Fusion,
		feedback:  deps.KnowledgeSpace.Feedback,
		delta:     deps.KnowledgeSpace.Delta,
		qa:        deps.KnowledgeSpace.QABridge,
	}
}

// Register wires the service into a registrar.
func Register(registrar grpc.ServiceRegistrar, srv *Server) {
	if registrar == nil || srv == nil || srv.svc == nil {
		return
	}
	knowledgev1.RegisterKnowledgeSpaceAdminServiceServer(registrar, srv)
	knowledgev1.RegisterKnowledgeSpaceQABridgeServiceServer(registrar, srv)
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
		switch {
		case errors.Is(err, ksvc.ErrInvalidInput):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case errors.Is(err, ksvc.ErrSpaceNotFound):
			return nil, status.Error(codes.NotFound, err.Error())
		default:
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return &knowledgev1.IngestionJobResponse{Job: toProtoIngestionJob(job)}, nil
}

func (s *Server) PublishFusionStrategy(ctx context.Context, req *knowledgev1.FusionStrategyRequest) (*knowledgev1.FusionStrategyResponse, error) {
	if s.fusion == nil {
		return nil, status.Error(codes.Unimplemented, "fusion service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	strategy, err := s.fusion.PublishStrategy(ctx, ksvc.PublishStrategyInput{
		SpaceID:         spaceID,
		Label:           req.GetLabel(),
		BM25Weight:      req.GetBm25Weight(),
		VectorWeight:    req.GetVectorWeight(),
		GraphConstraint: req.GetGraphConstraint(),
		RerankerModel:   req.GetRerankerModel(),
		ConflictPolicy:  req.GetConflictPolicy(),
	})
	if err != nil {
		return nil, mapFusionError(err)
	}
	return &knowledgev1.FusionStrategyResponse{Strategy: toProtoFusionStrategy(strategy)}, nil
}

func (s *Server) ListFusionStrategies(ctx context.Context, req *knowledgev1.ListFusionStrategiesRequest) (*knowledgev1.ListFusionStrategiesResponse, error) {
	if s.fusion == nil {
		return nil, status.Error(codes.Unimplemented, "fusion service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	strategies, err := s.fusion.ListStrategies(ctx, spaceID, int(req.GetLimit()))
	if err != nil {
		return nil, mapFusionError(err)
	}
	return &knowledgev1.ListFusionStrategiesResponse{
		Strategies: toProtoFusionStrategyList(strategies),
	}, nil
}

func (s *Server) RollbackFusionStrategy(ctx context.Context, req *knowledgev1.RollbackFusionStrategyRequest) (*knowledgev1.FusionStrategyResponse, error) {
	if s.fusion == nil {
		return nil, status.Error(codes.Unimplemented, "fusion service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	strategy, err := s.fusion.RollbackStrategy(ctx, ksvc.RollbackStrategyInput{
		SpaceID:    spaceID,
		StrategyID: req.GetStrategyId(),
	})
	if err != nil {
		return nil, mapFusionError(err)
	}
	return &knowledgev1.FusionStrategyResponse{Strategy: toProtoFusionStrategy(strategy)}, nil
}

func (s *Server) PlanRetrieval(ctx context.Context, req *knowledgev1.QARetrievalPlanRequest) (*knowledgev1.QARetrievalPlanResponse, error) {
	if s.qa == nil {
		return nil, status.Error(codes.Unavailable, "qa bridge not available")
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(req.GetTenantId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant id: %v", err)
	}
	out, err := s.qa.Plan(ctx, qaBridge.PlanInput{
		TenantID:        tenantID,
		Intent:          req.GetIntent(),
		DomainTags:      req.GetDomainTags(),
		SessionID:       req.GetSessionId(),
		LatencyBudgetMs: int(req.GetLatencyBudgetMs()),
	})
	if err != nil {
		switch {
		case errors.Is(err, qaBridge.ErrInvalidInput):
			return nil, status.Error(codes.InvalidArgument, err.Error())
		case errors.Is(err, qaBridge.ErrSpacesMissing):
			return nil, status.Error(codes.NotFound, err.Error())
		default:
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return &knowledgev1.QARetrievalPlanResponse{
		TenantId:        out.TenantID.String(),
		Intent:          out.Intent,
		DomainTags:      out.DomainTags,
		CandidateSpaces: toProtoCandidateSpaces(out.CandidateSpaces),
		Toolings:        toProtoToolings(out.Toolings),
		Telemetry: &knowledgev1.QATelemetry{
			TraceId:    out.TraceID,
			RecordedAt: timestamppb.New(out.RecordedAt),
		},
		DegradeCount:    int32(out.DegradeCount),
		SessionId:       out.SessionID,
		LatencyBudgetMs: int32(out.LatencyBudgetMs),
	}, nil
}

func (s *Server) UpsertMemorySnapshot(ctx context.Context, req *knowledgev1.QAMemorySnapshotRequest) (*knowledgev1.QAMemorySnapshotResponse, error) {
	if s.qa == nil {
		return nil, status.Error(codes.Unavailable, "qa bridge not available")
	}
	tenantID, err := uuid.Parse(strings.TrimSpace(req.GetTenantId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tenant id: %v", err)
	}
	out, err := s.qa.UpsertMemorySnapshot(ctx, qaBridge.MemoryInput{
		TenantID:  tenantID,
		SessionID: req.GetSessionId(),
		Updates:   fromProtoUpdates(req.GetUpdates()),
	})
	if err != nil {
		if errors.Is(err, qaBridge.ErrInvalidInput) {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &knowledgev1.QAMemorySnapshotResponse{
		TenantId:  out.TenantID.String(),
		SessionId: out.SessionID,
		Citations: toProtoCitations(out.Citations),
	}, nil
}

func (s *Server) SubmitFeedback(ctx context.Context, req *knowledgev1.FeedbackRequest) (*knowledgev1.FeedbackResponse, error) {
	if s.feedback == nil {
		return nil, status.Error(codes.Unimplemented, "feedback service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	chunkIDs := make([]uuid.UUID, 0, len(req.GetLinkedChunks()))
	for _, chunk := range req.GetLinkedChunks() {
		id, err := uuid.Parse(strings.TrimSpace(chunk))
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid chunk id: %v", err)
		}
		chunkIDs = append(chunkIDs, id)
	}
	caseModel, err := s.feedback.SubmitFeedback(ctx, ksvc.SubmitFeedbackInput{
		SpaceID:      spaceID,
		ReportedBy:   req.GetReportedBy(),
		Severity:     req.GetSeverity(),
		IssueType:    req.GetIssueType(),
		Notes:        req.GetNotes(),
		ToolTraceRef: req.GetToolTraceRef(),
		LinkedChunks: chunkIDs,
	})
	if err != nil {
		return nil, mapFeedbackError(err)
	}
	return &knowledgev1.FeedbackResponse{Case: toProtoFeedbackCase(caseModel)}, nil
}

func (s *Server) ListFeedbackCases(ctx context.Context, req *knowledgev1.ListFeedbackCasesRequest) (*knowledgev1.ListFeedbackCasesResponse, error) {
	if s.feedback == nil {
		return nil, status.Error(codes.Unimplemented, "feedback service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	cases, err := s.feedback.ListCases(ctx, spaceID, int(req.GetLimit()))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	resp := make([]*knowledgev1.FeedbackCase, 0, len(cases))
	for _, item := range cases {
		resp = append(resp, toProtoFeedbackCase(item))
	}
	return &knowledgev1.ListFeedbackCasesResponse{Cases: resp}, nil
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

func toProtoFeedbackCase(caseModel *models.FeedbackCase) *knowledgev1.FeedbackCase {
	if caseModel == nil {
		return nil
	}
	var chunks []string
	if len(caseModel.LinkedChunks) > 0 {
		_ = json.Unmarshal(caseModel.LinkedChunks, &chunks)
	}
	var slaDue *timestamppb.Timestamp
	if caseModel.SLADueAt != nil {
		slaDue = timestamppb.New(*caseModel.SLADueAt)
	}
	return &knowledgev1.FeedbackCase{
		CaseId:       caseModel.UUID.String(),
		SpaceId:      caseModel.SpaceUUID.String(),
		Status:       caseModel.Status,
		Severity:     caseModel.Severity,
		IssueType:    caseModel.IssueType,
		LinkedChunks: chunks,
		ReportedBy:   caseModel.ReportedBy,
		Notes:        caseModel.Notes,
		ToolTraceRef: caseModel.ToolTraceRef,
		QualityScore: caseModel.QualityScore,
		SlaDueAt:     slaDue,
		CreatedAt:    timestamppb.New(caseModel.CreatedAt),
		UpdatedAt:    timestamppb.New(caseModel.UpdatedAt),
	}
}

func toProtoFusionStrategy(strategy *models.FusionStrategyVersion) *knowledgev1.FusionStrategy {
	if strategy == nil {
		return nil
	}
	var publishedAt *timestamppb.Timestamp
	if strategy.PublishedAt != nil {
		publishedAt = timestamppb.New(*strategy.PublishedAt)
	}
	return &knowledgev1.FusionStrategy{
		StrategyId:      strategy.ID,
		SpaceId:         strategy.SpaceUUID.String(),
		Label:           strategy.Label,
		Bm25Weight:      strategy.BM25Weight,
		VectorWeight:    strategy.VectorWeight,
		GraphConstraint: strategy.GraphConstraint,
		RerankerModel:   strategy.RerankerModel,
		ConflictPolicy:  strategy.ConflictPolicy,
		DeploymentState: protoDeploymentState(strategy.DeploymentState),
		PublishedAt:     publishedAt,
	}
}

func toProtoFusionStrategyList(list []*models.FusionStrategyVersion) []*knowledgev1.FusionStrategy {
	out := make([]*knowledgev1.FusionStrategy, 0, len(list))
	for _, item := range list {
		out = append(out, toProtoFusionStrategy(item))
	}
	return out
}

func toProtoCandidateSpaces(spaces []qaBridge.CandidateSpace) []*knowledgev1.QACandidateSpace {
	out := make([]*knowledgev1.QACandidateSpace, 0, len(spaces))
	for _, item := range spaces {
		out = append(out, &knowledgev1.QACandidateSpace{
			SpaceId:          item.SpaceID.String(),
			SpaceName:        item.SpaceName,
			Strategy:         item.Strategy,
			CitationCoverage: item.CitationCoverage,
			DegradeReason:    item.DegradeReason,
		})
	}
	return out
}

func toProtoToolings(items []toolchain.Metadata) []*knowledgev1.QAToolMetadata {
	out := make([]*knowledgev1.QAToolMetadata, 0, len(items))
	for _, item := range items {
		out = append(out, &knowledgev1.QAToolMetadata{
			ToolId:   item.ToolID,
			Name:     item.Name,
			Category: item.Category,
			Endpoint: item.Endpoint,
		})
	}
	return out
}

func fromProtoUpdates(items []*knowledgev1.QAMemoryUpdate) []context_snapshot.Citation {
	out := make([]context_snapshot.Citation, 0, len(items))
	for _, item := range items {
		out = append(out, context_snapshot.Citation{
			ChunkID:     item.GetChunkId(),
			SpaceID:     item.GetSpaceId(),
			Status:      item.GetStatus(),
			Citations:   item.GetCitations(),
			SourceType:  item.GetSourceType(),
			Confidence:  item.GetConfidence(),
			DeltaReason: item.GetDeltaReason(),
		})
	}
	return out
}

func toProtoCitations(items []context_snapshot.Citation) []*knowledgev1.QACitationSummary {
	out := make([]*knowledgev1.QACitationSummary, 0, len(items))
	for _, item := range items {
		out = append(out, &knowledgev1.QACitationSummary{
			ChunkId:     item.ChunkID,
			SpaceId:     item.SpaceID,
			Citations:   item.Citations,
			Status:      item.Status,
			SourceType:  item.SourceType,
			Confidence:  item.Confidence,
			DeltaReason: item.DeltaReason,
		})
	}
	return out
}

func protoDeploymentState(state string) knowledgev1.FusionStrategy_DeploymentState {
	switch state {
	case models.FusionDeploymentActive:
		return knowledgev1.FusionStrategy_DEPLOYMENT_STATE_ACTIVE
	case models.FusionDeploymentRollback:
		return knowledgev1.FusionStrategy_DEPLOYMENT_STATE_ROLLBACK
	case models.FusionDeploymentDraft:
		return knowledgev1.FusionStrategy_DEPLOYMENT_STATE_DRAFT
	default:
		return knowledgev1.FusionStrategy_DEPLOYMENT_STATE_UNSPECIFIED
	}
}

func mapFusionError(err error) error {
	switch {
	case errors.Is(err, ksvc.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ksvc.ErrSpaceNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ksvc.ErrFusionConflict):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, ksvc.ErrFusionStrategyNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func mapDeltaError(err error) error {
	switch {
	case errors.Is(err, ksdelta.ErrInvalidInput), errors.Is(err, ksdelta.ErrUnknownSource):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ksdelta.ErrSpaceNotFound), errors.Is(err, ksdelta.ErrJobNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, ksdelta.ErrPartialReleaseDenied):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func toProtoDeltaJob(job *models.DeltaJob) *knowledgev1.DeltaJob {
	if job == nil {
		return nil
	}
	var report string
	if len(job.Report) > 0 {
		report = string(job.Report)
	}
	return &knowledgev1.DeltaJob{
		JobId:          job.UUID.String(),
		SpaceId:        job.SpaceUUID.String(),
		Source:         job.Source,
		Status:         job.Status,
		ApprovalState:  job.ApprovalState,
		DiffAccuracy:   job.DiffAccuracy,
		PartialRelease: job.PartialRelease,
		RollbackCount:  int32(job.RollbackCount),
		CreatedAt:      timestamppb.New(job.CreatedAt),
		PublishedAt:    toTimestamp(job.PublishedAt),
		ReportJson:     report,
	}
}

func toTimestamp(ts *time.Time) *timestamppb.Timestamp {
	if ts == nil {
		return nil
	}
	return timestamppb.New(ts.UTC())
}

func mapFeedbackError(err error) error {
	switch {
	case errors.Is(err, ksvc.ErrInvalidInput):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ksvc.ErrSpaceNotFound):
		return status.Error(codes.NotFound, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

func (s *Server) StartDeltaJob(ctx context.Context, req *knowledgev1.StartDeltaJobRequest) (*knowledgev1.StartDeltaJobResponse, error) {
	if s.delta == nil {
		return nil, status.Error(codes.Unimplemented, "delta service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	job, err := s.delta.StartJob(ctx, ksdelta.StartJobInput{
		SpaceID:      spaceID,
		Source:       req.GetSource(),
		PackageURI:   req.GetPackageUri(),
		DiffAccuracy: req.GetDiffAccuracy(),
		RequestedBy:  req.GetRequestedBy(),
		Notes:        req.GetNotes(),
	})
	if err != nil {
		return nil, mapDeltaError(err)
	}
	return &knowledgev1.StartDeltaJobResponse{Job: toProtoDeltaJob(job)}, nil
}

func (s *Server) GetDeltaReport(ctx context.Context, req *knowledgev1.GetDeltaReportRequest) (*knowledgev1.GetDeltaReportResponse, error) {
	if s.delta == nil {
		return nil, status.Error(codes.Unimplemented, "delta service not available")
	}
	jobID, err := uuid.Parse(strings.TrimSpace(req.GetJobId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid job id: %v", err)
	}
	job, err := s.delta.GetReport(ctx, jobID)
	if err != nil {
		return nil, mapDeltaError(err)
	}
	return &knowledgev1.GetDeltaReportResponse{Job: toProtoDeltaJob(job)}, nil
}

func (s *Server) PublishDeltaJob(ctx context.Context, req *knowledgev1.PublishDeltaJobRequest) (*knowledgev1.PublishDeltaJobResponse, error) {
	if s.delta == nil {
		return nil, status.Error(codes.Unimplemented, "delta service not available")
	}
	jobID, err := uuid.Parse(strings.TrimSpace(req.GetJobId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid job id: %v", err)
	}
	job, err := s.delta.Publish(ctx, ksdelta.PublishJobInput{
		JobID:          jobID,
		Decision:       req.GetDecision(),
		ApprovedBy:     req.GetApprovedBy(),
		DiffAccuracy:   req.GetDiffAccuracy(),
		PartialRelease: req.GetPartialRelease(),
	})
	if err != nil {
		return nil, mapDeltaError(err)
	}
	return &knowledgev1.PublishDeltaJobResponse{Job: toProtoDeltaJob(job)}, nil
}

func (s *Server) RollbackDelta(ctx context.Context, req *knowledgev1.RollbackDeltaRequest) (*knowledgev1.RollbackDeltaResponse, error) {
	if s.delta == nil {
		return nil, status.Error(codes.Unimplemented, "delta service not available")
	}
	jobID, err := uuid.Parse(strings.TrimSpace(req.GetJobId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid job id: %v", err)
	}
	job, err := s.delta.Rollback(ctx, ksdelta.RollbackInput{
		JobID:       jobID,
		RequestedBy: req.GetRequestedBy(),
		Reason:      req.GetReason(),
	})
	if err != nil {
		return nil, mapDeltaError(err)
	}
	return &knowledgev1.RollbackDeltaResponse{Job: toProtoDeltaJob(job)}, nil
}
