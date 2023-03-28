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
	roles := l.svcCtx.PowerX.Auth.FindAllRoles(l.ctx)

	var roleList []types.AdminRole
	for _, role := range roles {
		var api []types.AdminAPI
		for _, adminAPI := range role.AdminAPI {
			api = append(api, types.AdminAPI{
				Id:   adminAPI.ID,
				API:  adminAPI.API,
				Name: adminAPI.Name,
				Desc: adminAPI.Desc,
			})
		}

		var menus []string
		for _, menu := range role.MenuNames {
			menus = append(menus, menu.MenuName)
		}

		roleList = append(roleList, types.AdminRole{
			RoleCode:   role.RoleCode,
			Name:       role.Name,
			Desc:       role.Desc,
			IsReserved: role.IsReserved,
			APIList:    api,
			MenuNames:  menus,
		})
	}

	return &types.ListRolesReply{
		List: roleList,
	}, nil
}
