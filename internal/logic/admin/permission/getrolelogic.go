package permission

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRoleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRoleLogic {
	return &GetRoleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetRoleLogic) GetRole(req *types.GetRoleRequest) (resp *types.GetRoleReply, err error) {
	role, err := l.svcCtx.PowerX.AdminAuthorization.FindOneRoleByRoleCode(l.ctx, req.RoleCode)
	if err != nil {
		return nil, err
	}

	api := make([]types.AdminAPI, 0, len(role.AdminAPI))
	for _, adminAPI := range role.AdminAPI {
		api = append(api, types.AdminAPI{
			Id:     adminAPI.Id,
			API:    adminAPI.API,
			Method: adminAPI.Method,
			Name:   adminAPI.Name,
			Desc:   adminAPI.Desc,
		})
	}

	menus := make([]string, 0, len(role.MenuNames))
	for _, menu := range role.MenuNames {
		menus = append(menus, menu.MenuName)
	}

	return &types.GetRoleReply{
		AdminRole: &types.AdminRole{
			RoleCode:   role.RoleCode,
			Name:       role.Name,
			Desc:       role.Desc,
			IsReserved: role.IsReserved,
			APIList:    api,
			MenuNames:  menus,
		},
	}, nil
}
