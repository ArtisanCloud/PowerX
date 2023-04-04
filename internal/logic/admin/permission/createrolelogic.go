package permission

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/uc/powerx"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateRoleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateRoleLogic {
	return &CreateRoleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateRoleLogic) CreateRole(req *types.CreateRoleRequest) (resp *types.CreateRoleReply, err error) {
	var apiList []*powerx.AdminAPI
	for _, id := range req.APIIds {
		apiList = append(apiList, &powerx.AdminAPI{Model: types.Model{ID: id}})
	}

	var menuList []*powerx.AdminRoleMenuName
	for _, name := range req.MenuNames {
		menuList = append(menuList, &powerx.AdminRoleMenuName{MenuName: name})
	}

	role := powerx.AdminRole{
		RoleCode:  req.RoleCode,
		Name:      req.Name,
		Desc:      req.Desc,
		AdminAPI:  apiList,
		MenuNames: menuList,
	}

	l.svcCtx.PowerX.AdminAuthorization.CreateRole(l.ctx, &role)

	return &types.CreateRoleReply{
		RoleCode: role.RoleCode,
	}, nil
}
