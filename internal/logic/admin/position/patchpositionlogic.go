package position

import (
	"PowerX/internal/model/option"
	"PowerX/internal/model/organization"
	"PowerX/internal/model/permission"
	"PowerX/pkg/slicex"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchPositionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchPositionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchPositionLogic {
	return &PatchPositionLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchPositionLogic) PatchPosition(req *types.PatchPositionRequest) (resp *types.PatchPositionReply, err error) {
	patch := &organization.Position{
		Name:  req.Name,
		Desc:  req.Desc,
		Level: req.Level,
	}
	if len(req.RoleCodes) > 0 {
		roles := make([]*permission.AdminRole, 0, len(req.RoleCodes))
		for _, code := range req.RoleCodes {
			roles = append(roles, &permission.AdminRole{
				RoleCode: code,
			})
		}
		patch.Roles = roles
	}

	// 更新职位
	err = l.svcCtx.PowerX.Organization.PatchPosition(l.ctx, req.Id, patch)
	if err != nil {
		return
	}

	// 查询该职位的员工
	page := l.svcCtx.PowerX.Organization.FindManyUsersPage(l.ctx, &option.FindManyUsersOption{
		PositionIDs: []int64{req.Id},
	})
	// pluck user id
	userIDs := slicex.SlicePluck(page.List, func(item *organization.User) int64 {
		return item.Id
	})

	// 更新员工的角色
	for _, id := range userIDs {
		err = l.svcCtx.PowerX.AdminAuthorization.ReplaceUserRoles(l.ctx, id, req.RoleCodes)
		if err != nil {
			return
		}
	}
	return &types.PatchPositionReply{
		Position: &types.Position{
			Id:   patch.Id,
			Name: patch.Name,
			Desc: patch.Desc,
			// roles
		},
	}, nil
}
