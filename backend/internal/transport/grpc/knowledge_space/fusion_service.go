package knowledge_space

import (
	"context"
	"strings"

	knowledgev1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/knowledge/v1"
	ksvc "github.com/ArtisanCloud/PowerX/internal/service/knowledge_space"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
