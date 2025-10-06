package iam

import (
    "context"
    "errors"
    "strconv"
    "strings"

    commonv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/common/v1"
    iamv1 "github.com/ArtisanCloud/PowerX/api/grpc/gen/go/powerx/iam/v1"
    "github.com/ArtisanCloud/PowerX/internal/app/shared"
    svciam "github.com/ArtisanCloud/PowerX/internal/service/iam"

    "google.golang.org/grpc/metadata"
    "gorm.io/gorm"
)

// 编译期保证接口实现
var _ iamv1.MemberServiceServer = (*MemberServer)(nil)

// MemberServer 是 gRPC 适配层：解析/校验/映射 → 调用应用服务
type MemberServer struct {
	iamv1.UnimplementedMemberServiceServer
	deps      *shared.Deps
	memberSvc *svciam.MemberService // ✅ 缓存起来

}

// 依赖注入
func NewMemberServer(deps *shared.Deps) *MemberServer {
	return &MemberServer{
		deps:      deps,
		memberSvc: svciam.NewMemberService(deps.DB),
	}
}

// ========== Helpers ==========

// 从 RequestContext 或 Metadata 里兜底拿 tenant_id
func tenantIDFrom(ctx context.Context, rc *commonv1.RequestContext) int64 {
	if rc != nil && rc.TenantId > 0 {
		return rc.TenantId
	}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for _, k := range []string{"x-powerx-tenant-id", "tenant-id", "x-tenant-id"} {
			if vals := md.Get(k); len(vals) > 0 {
				if v, err := strconv.ParseInt(vals[0], 10, 64); err == nil && v > 0 {
					return v
				}
			}
		}
	}
	return 0
}

// PageRequest(offset, page_size) → (page, pageSize) ；page 从 1 开始
func pageFrom(pr *commonv1.PageRequest) (page, size int) {
	if pr == nil {
		return 1, 20
	}
	size = int(pr.GetPageSize())
	if size <= 0 {
		size = 20
	}
	off := int(pr.GetOffset())
	page = 1
	if off > 0 {
		page = off/size + 1
	}
	return
}

func toPBRef(tid uint64, entity string, id uint64, prn string, humanKey string) *commonv1.ResourceRef {
	return &commonv1.ResourceRef{
		TenantId: int64(tid),
		Domain:   "iam",
		Entity:   entity,
		Id:       strconv.FormatUint(id, 10),
		Prn:      prn,      // 可为空
		HumanKey: humanKey, // 可为空
	}
}

// 领域对象 → PB
func toPBMember(w svciam.MemberWithProfile) *iamv1.Member {
	m := w.Member
	pb := &iamv1.Member{
		Ref:         toPBRef(m.TenantID, "member", m.ID, "", ""),
		UserId:      m.UserID,
		Username:    m.Username,
		DisplayName: m.DisplayName,
		AvatarUrl:   m.AvatarURL,
		Status:      iamv1.MemberStatus(m.Status), // 模型里是 int16
	}
	// 如你的 proto 里有部门 ID 列表字段（例如 DepartmentIds），可按需填充：
	// pb.DepartmentIds = append([]uint64(nil), w.DeptIDs...)
	return pb
}

// ========== RPC 实现 ==========

func (s *MemberServer) ListMembers(ctx context.Context, req *iamv1.ListMembersRequest) (*iamv1.ListMembersResponse, error) {
    tid := tenantIDFrom(ctx, req.GetCtx())
    if tid == 0 {
        return &iamv1.ListMembersResponse{Meta: badMeta(ctx, 400, "tenant_id required", req.GetCtx().GetRequestId()), Data: &iamv1.ListMembersData{Items: []*iamv1.Member{}, Page: &commonv1.PageResponse{}}}, nil
    }

	page, size := pageFrom(req.GetPage())
	sortOrder := "desc"
	if req.GetAsc() {
		sortOrder = "asc"
	}
	opt := svciam.ListMembersOption{
		TenantID: uint64(tid),
		Keyword:  strings.TrimSpace(req.GetKeyword()),
		Status: func() *int16 {
			if req.GetStatus() == 0 {
				return nil
			}
			v := int16(req.GetStatus())
			return &v
		}(),
		DeptID: func() *uint64 {
			if req.GetDepartmentId() == 0 {
				return nil
			}
			id := uint64(req.GetDepartmentId())
			return &id
		}(),
		Recursive: false, // 如有需要可从 req 带一个 bool
		Page:      page,
		PageSize:  size,
		SortBy:    strings.TrimSpace(req.GetOrderBy()),
		SortOrder: sortOrder,
	}

    rows, total, err := s.memberSvc.ListMembers(ctx, opt)
    if err != nil {
        return &iamv1.ListMembersResponse{Meta: badMeta(ctx, 500, "list members: "+err.Error(), req.GetCtx().GetRequestId())}, nil
    }

	items := make([]*iamv1.Member, 0, len(rows))
	for _, w := range rows {
		items = append(items, toPBMember(w))
	}
    return &iamv1.ListMembersResponse{
        Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
        Data: &iamv1.ListMembersData{Items: items, Page: &commonv1.PageResponse{Total: total}},
    }, nil
}

func (s *MemberServer) GetMember(ctx context.Context, req *iamv1.GetMemberRequest) (*iamv1.GetMemberResponse, error) {
	tid := tenantIDFrom(ctx, req.GetCtx())
    if tid == 0 {
        return &iamv1.GetMemberResponse{Meta: badMeta(ctx, 400, "tenant_id required", req.GetCtx().GetRequestId())}, nil
    }

	// 仅实现按 ID 查询（如果你的 proto 还有 username/ref 等 selector，可按需扩展）
	id := req.GetId()
    if id == 0 {
        return &iamv1.GetMemberResponse{Meta: badMeta(ctx, 400, "id required", req.GetCtx().GetRequestId())}, nil
    }

    w, err := s.memberSvc.GetMember(ctx, uint64(tid), uint64(id))
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return &iamv1.GetMemberResponse{Meta: badMeta(ctx, 404, "member not found", req.GetCtx().GetRequestId())}, nil
    }
    if err != nil {
        return &iamv1.GetMemberResponse{Meta: badMeta(ctx, 500, "get member: "+err.Error(), req.GetCtx().GetRequestId())}, nil
    }

    return &iamv1.GetMemberResponse{
        Meta: okMeta(ctx, req.GetCtx().GetRequestId()),
        Data: &iamv1.GetMemberData{Member: toPBMember(*w)},
    }, nil
}

func (s *MemberServer) BatchGetMembers(ctx context.Context, req *iamv1.BatchGetMembersRequest) (*iamv1.BatchGetMembersResponse, error) {
	tid := tenantIDFrom(ctx, req.GetCtx())
    if tid == 0 {
        return &iamv1.BatchGetMembersResponse{Meta: badMeta(ctx, 400, "tenant_id required", req.GetCtx().GetRequestId()), Data: &iamv1.BatchGetMembersData{Members: []*iamv1.Member{}}}, nil
    }
	ids := req.GetIds()
    if len(ids) == 0 {
        return &iamv1.BatchGetMembersResponse{Meta: okMeta(ctx, req.GetCtx().GetRequestId()), Data: &iamv1.BatchGetMembersData{Members: []*iamv1.Member{}}}, nil
    }

	out := make([]*iamv1.Member, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		w, err := s.memberSvc.GetMember(ctx, uint64(tid), uint64(id))
		if err != nil {
			// 忽略不存在或错误的 id，按需也可累计后一次性返回错误
			continue
		}
		out = append(out, toPBMember(*w))
	}
    return &iamv1.BatchGetMembersResponse{Meta: okMeta(ctx, req.GetCtx().GetRequestId()), Data: &iamv1.BatchGetMembersData{Members: out}}, nil
}
