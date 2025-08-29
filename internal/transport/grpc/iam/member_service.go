package iam

import (
	commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
	"github.com/ArtisanCloud/PowerX/internal/app/shared"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"context"

	orgv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/iam/v1"
)

// 确保实现了接口
var _ orgv1.MemberServiceServer = (*MemberServer)(nil)

// MemberServer 是 gRPC 适配层：解析/校验/映射 → 调用应用服务（你已有的 internal/service/...）
type MemberServer struct {
	orgv1.UnimplementedMemberServiceServer
	deps *shared.Deps
}

// 依赖注入
func NewMemberServer(deps *shared.Deps) *MemberServer {
	return &MemberServer{deps: deps}
}

// ====== RPC 实现（先给最小可跑，返回空结果；后续你把 TODO 替换为真实调用） ======

func (s *MemberServer) GetMember(ctx context.Context, req *orgv1.GetMemberRequest) (*orgv1.GetMemberResponse, error) {
	if req.GetCtx().GetTenantId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "tenant_id required")
	}
	// TODO: 调用你现有的应用服务：s.deps.<xxxService>.GetMember(...)
	//       选择器可由 req.Selector 决定（id/prn/username）
	return nil, status.Error(codes.Unimplemented, "GetMember not implemented yet")
}

func (s *MemberServer) BatchGetMembers(ctx context.Context, req *orgv1.BatchGetMembersRequest) (*orgv1.BatchGetMembersResponse, error) {
	if req.GetCtx().GetTenantId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "tenant_id required")
	}
	// TODO: s.deps.<xxxService>.BatchGetMembers(...)
	return &orgv1.BatchGetMembersResponse{Members: []*orgv1.Member{}}, nil
}

func (s *MemberServer) ListMembers(ctx context.Context, req *orgv1.ListMembersRequest) (*orgv1.ListMembersResponse, error) {
	if req.GetCtx().GetTenantId() == 0 {
		return nil, status.Error(codes.InvalidArgument, "tenant_id required")
	}
	// TODO: 构造过滤条件 → 调用应用服务（例如 s.deps.<OrgService>.ListMembers(filter)）
	// 注意分页：req.Page.PageSize / req.Page.Offset
	// 注意排序：req.OrderBy / req.Asc
	return &orgv1.ListMembersResponse{
		Items: []*orgv1.Member{},
		Page:  &commonv1.PageResponse{}, // 需要总数时填充 Total
	}, nil
}
