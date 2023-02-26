package permission

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListRolesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListRolesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListRolesLogic {
	return &ListRolesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListRolesLogic) ListRoles() (resp *types.ListRolesReply, err error) {
	roles := l.svcCtx.UC.Auth.FindManyAuthRoles(l.ctx)
	var list []types.AuthRole
	for _, role := range roles {
		list = append(list, types.AuthRole{
			RoleCode:   role.RoleCode,
			Name:       role.Name,
			Desc:       role.Desc,
			IsReserved: role.IsReserved,
		})
	}

	resp = &types.ListRolesReply{
		List: list,
	}
	return
}
