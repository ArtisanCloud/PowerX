package knowledge_space

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	models "github.com/ArtisanCloud/PowerX/pkg/corex/db/persistence/model/knowledge"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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

func (s *Server) CloseFeedbackCase(ctx context.Context, req *knowledgev1.CloseFeedbackCaseRequest) (*knowledgev1.FeedbackResponse, error) {
	if s.feedback == nil {
		return nil, status.Error(codes.Unimplemented, "feedback service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	caseID, err := uuid.Parse(strings.TrimSpace(req.GetCaseId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid case id: %v", err)
	}
	caseModel, err := s.feedback.CloseCase(ctx, ksvc.FeedbackCaseUpdateInput{
		SpaceID: spaceID,
		CaseID:  caseID,
		Actor:   req.GetRequestedBy(),
		Notes:   req.GetResolutionNotes(),
	})
	if err != nil {
		return nil, mapFeedbackError(err)
	}
	return &knowledgev1.FeedbackResponse{Case: toProtoFeedbackCase(caseModel)}, nil
}

func (s *Server) EscalateFeedbackCase(ctx context.Context, req *knowledgev1.EscalateFeedbackCaseRequest) (*knowledgev1.FeedbackResponse, error) {
	if s.feedback == nil {
		return nil, status.Error(codes.Unimplemented, "feedback service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	caseID, err := uuid.Parse(strings.TrimSpace(req.GetCaseId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid case id: %v", err)
	}
	caseModel, err := s.feedback.EscalateCase(ctx, ksvc.FeedbackCaseUpdateInput{
		SpaceID: spaceID,
		CaseID:  caseID,
		Actor:   req.GetRequestedBy(),
		Notes:   req.GetReason(),
	})
	if err != nil {
		return nil, mapFeedbackError(err)
	}
	return &knowledgev1.FeedbackResponse{Case: toProtoFeedbackCase(caseModel)}, nil
}

func (s *Server) ReprocessFeedbackCase(ctx context.Context, req *knowledgev1.ReprocessFeedbackCaseRequest) (*knowledgev1.FeedbackResponse, error) {
	if s.feedback == nil {
		return nil, status.Error(codes.Unimplemented, "feedback service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	caseID, err := uuid.Parse(strings.TrimSpace(req.GetCaseId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid case id: %v", err)
	}
	caseModel, err := s.feedback.ReprocessCase(ctx, spaceID, caseID, req.GetRequestedBy())
	if err != nil {
		return nil, mapFeedbackError(err)
	}
	return &knowledgev1.FeedbackResponse{Case: toProtoFeedbackCase(caseModel)}, nil
}

func (s *Server) RollbackFeedbackCase(ctx context.Context, req *knowledgev1.RollbackFeedbackCaseRequest) (*knowledgev1.FeedbackResponse, error) {
	if s.feedback == nil {
		return nil, status.Error(codes.Unimplemented, "feedback service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	caseID, err := uuid.Parse(strings.TrimSpace(req.GetCaseId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid case id: %v", err)
	}
	caseModel, err := s.feedback.RollbackCase(ctx, spaceID, caseID, req.GetRequestedBy(), req.GetReason())
	if err != nil {
		return nil, mapFeedbackError(err)
	}
	return &knowledgev1.FeedbackResponse{Case: toProtoFeedbackCase(caseModel)}, nil
}

func (s *Server) ExportFeedbackCases(ctx context.Context, req *knowledgev1.ExportFeedbackCasesRequest) (*knowledgev1.ExportFeedbackCasesResponse, error) {
	if s.feedback == nil {
		return nil, status.Error(codes.Unimplemented, "feedback service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}
	export, err := s.feedback.ExportCases(ctx, spaceID, ksvc.ListFeedbackFilter{
		Status:   req.GetStatus(),
		Severity: req.GetSeverity(),
		Limit:    int(req.GetLimit()),
	})
	if err != nil {
		return nil, mapFeedbackError(err)
	}
	resp := make([]*knowledgev1.FeedbackCase, 0, len(export.Cases))
	for _, item := range export.Cases {
		resp = append(resp, toProtoFeedbackCase(item))
	}
	payload, _ := json.Marshal(export)
	return &knowledgev1.ExportFeedbackCasesResponse{
		Cases:     resp,
		ExportJson: string(payload),
	}, nil
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
	var escalatedAt *timestamppb.Timestamp
	if caseModel.EscalatedAt != nil {
		escalatedAt = timestamppb.New(*caseModel.EscalatedAt)
	}
	var closedAt *timestamppb.Timestamp
	if caseModel.ClosedAt != nil {
		closedAt = timestamppb.New(*caseModel.ClosedAt)
	}
	return &knowledgev1.FeedbackCase{
		CaseId:          caseModel.UUID.String(),
		SpaceId:         caseModel.SpaceUUID.String(),
		Status:          caseModel.Status,
		Severity:        caseModel.Severity,
		IssueType:       caseModel.IssueType,
		LinkedChunks:    chunks,
		ReportedBy:      caseModel.ReportedBy,
		Notes:           caseModel.Notes,
		ToolTraceRef:    caseModel.ToolTraceRef,
		QualityScore:    caseModel.QualityScore,
		SlaDueAt:        slaDue,
		CreatedAt:       timestamppb.New(caseModel.CreatedAt),
		UpdatedAt:       timestamppb.New(caseModel.UpdatedAt),
		EscalatedAt:     escalatedAt,
		ClosedAt:        closedAt,
		ResolutionNotes: caseModel.ResolutionNotes,
	}
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
