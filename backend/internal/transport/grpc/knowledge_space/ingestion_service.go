package knowledge_space

import (
	"context"
	"errors"
	"net/http"
	"strings"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/ArtisanCloud/PowerX/pkg/dto"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) TriggerIngestion(ctx context.Context, req *knowledgev1.IngestionJobRequest) (*knowledgev1.IngestionJobResponse, error) {
	if s.ingestion == nil {
		return nil, status.Error(codes.Unimplemented, "ingestion service not available")
	}
	spaceID, err := uuid.Parse(strings.TrimSpace(req.GetSpaceId()))
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid space id: %v", err)
	}

	format := strings.TrimSpace(req.GetFormat())
	if format == "" {
		format = strings.TrimSpace(req.GetSourceType())
	}
	if format == "" {
		return nil, status.Error(codes.InvalidArgument, "missing format/source_type")
	}

	job, err := s.ingestion.Trigger(ctx, ksvc.TriggerIngestionInput{
		SpaceID:          spaceID,
		Format:           format,
		SourceURI:        req.GetSourceUri(),
		IngestionProfile: req.GetIngestionProfile(),
		ProcessorProfile: req.GetProcessorProfile(),
		OCRRequired:      req.GetOcrRequired(),
		MaskingProfile:   req.GetMaskingProfile(),
		Priority:         req.GetPriority(),
		RequestedBy:      req.GetRequestedBy(),
	})
	if err != nil {
		var appErr *dto.AppError
		if errors.As(err, &appErr) {
			return nil, status.Error(codeFromHTTP(appErr.HTTPCode), appErr.Message)
		}
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

func codeFromHTTP(code int) codes.Code {
	switch code {
	case http.StatusBadRequest:
		return codes.InvalidArgument
	case http.StatusUnauthorized:
		return codes.Unauthenticated
	case http.StatusForbidden:
		return codes.PermissionDenied
	case http.StatusNotFound:
		return codes.NotFound
	case http.StatusConflict:
		return codes.AlreadyExists
	case http.StatusPreconditionFailed:
		return codes.FailedPrecondition
	case http.StatusTooManyRequests:
		return codes.ResourceExhausted
	default:
		return codes.Internal
	}
}
