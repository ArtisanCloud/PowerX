package iam

// internal/transport/grpc/iam/team_handler.go

import (
	"context"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	orgv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/iam/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
)

// 编译期保证接口实现
var _ orgv1.TeamServiceServer = (*TeamServer)(nil)

type TeamServer struct {
	orgv1.UnimplementedTeamServiceServer
	deps *shared.Deps
}

func NewTeamServer(deps *shared.Deps) *TeamServer {
	return &TeamServer{deps: deps}
}

func (s *TeamServer) GetTeam(ctx context.Context, req *orgv1.GetTeamRequest) (*orgv1.GetTeamResponse, error) {
	if _, err := tenantUUIDFromContext(ctx, req.GetCtx()); err != nil {
		return &orgv1.GetTeamResponse{Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId())}, nil
	}
	// TODO: 实现真实查询
	return &orgv1.GetTeamResponse{Meta: badMeta(ctx, 501, "GetTeam not implemented yet", req.GetCtx().GetRequestId())}, nil
}

func (s *TeamServer) ListTeams(ctx context.Context, req *orgv1.ListTeamsRequest) (*orgv1.ListTeamsResponse, error) {
	if _, err := tenantUUIDFromContext(ctx, req.GetCtx()); err != nil {
		return &orgv1.ListTeamsResponse{Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId())}, nil
	}
	// TODO: 实现真实查询
	return &orgv1.ListTeamsResponse{
		Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
		Data: &orgv1.ListTeamsData{Items: []*orgv1.Team{}, Page: &commonv1.PageResponse{}},
	}, nil
}

func (s *TeamServer) ListTeamMembers(ctx context.Context, req *orgv1.ListTeamMembersRequest) (*orgv1.ListTeamMembersResponse, error) {
	if _, err := tenantUUIDFromContext(ctx, req.GetCtx()); err != nil {
		return &orgv1.ListTeamMembersResponse{Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId())}, nil
	}
	// TODO: 实现真实查询
	return &orgv1.ListTeamMembersResponse{
		Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
		Data: &orgv1.ListTeamMembersData{Items: []*orgv1.TeamMember{}, Page: &commonv1.PageResponse{}},
	}, nil
}

func (s *TeamServer) ListMemberTeams(ctx context.Context, req *orgv1.ListMemberTeamsRequest) (*orgv1.ListMemberTeamsResponse, error) {
	if _, err := tenantUUIDFromContext(ctx, req.GetCtx()); err != nil {
		return &orgv1.ListMemberTeamsResponse{Meta: badMeta(ctx, 400, err.Error(), req.GetCtx().GetRequestId())}, nil
	}
	// TODO: 实现真实查询
	return &orgv1.ListMemberTeamsResponse{
		Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
		Data: &orgv1.ListMemberTeamsData{Items: []*orgv1.Team{}, Page: &commonv1.PageResponse{}},
	}, nil
}
