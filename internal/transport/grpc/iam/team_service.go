package iam

import (
	"context"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"

	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	orgv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/iam/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	if req.GetCtx().GetTenantId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "tenant_id required")
	}
	// TODO: s.deps.<OrgService>.GetTeam(...)
	return nil, status.Error(codes.Unimplemented, "GetTeam not implemented yet")
}

func (s *TeamServer) ListTeams(ctx context.Context, req *orgv1.ListTeamsRequest) (*orgv1.ListTeamsResponse, error) {
	if req.GetCtx().GetTenantId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "tenant_id required")
	}
	// TODO: s.deps.<OrgService>.ListTeams(filter)
	return &orgv1.ListTeamsResponse{
		Items: []*orgv1.Team{},
		Page:  &commonv1.PageResponse{},
	}, nil
}

func (s *TeamServer) ListTeamMembers(ctx context.Context, req *orgv1.ListTeamMembersRequest) (*orgv1.ListTeamMembersResponse, error) {
	if req.GetCtx().GetTenantId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "tenant_id required")
	}
	// TODO: s.deps.<OrgService>.ListTeamMembers(teamID/prn, keyword/role/status, page)
	return &orgv1.ListTeamMembersResponse{
		Items: []*orgv1.TeamMember{},
		Page:  &commonv1.PageResponse{},
	}, nil
}

func (s *TeamServer) ListMemberTeams(ctx context.Context, req *orgv1.ListMemberTeamsRequest) (*orgv1.ListMemberTeamsResponse, error) {
	if req.GetCtx().GetTenantId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "tenant_id required")
	}
	// TODO: s.deps.<OrgService>.ListMemberTeams(memberID/prn/username, page)
	return &orgv1.ListMemberTeamsResponse{
		Items: []*orgv1.Team{},
		Page:  &commonv1.PageResponse{},
	}, nil
}
