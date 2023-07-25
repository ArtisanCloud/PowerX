package permission

import (
	"PowerX/internal/model"
	"PowerX/internal/model/permission"
	"PowerX/internal/svc"
	"PowerX/internal/types"
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
	var apiList []*permission.AdminAPI
	for _, id := range req.APIIds {
		apiList = append(apiList, &permission.AdminAPI{CommonModel: model.CommonModel{Id: id}})
	}

	var menuList []*permission.AdminRoleMenuName
	for _, name := range req.MenuNames {
		menuList = append(menuList, &permission.AdminRoleMenuName{MenuName: name})
	}

	role := permission.AdminRole{
		RoleCode:  req.RoleCode,
		Name:      req.Name,
		Desc:      req.Desc,
		AdminAPI:  apiList,
		MenuNames: menuList,
	}

	if err := l.svcCtx.PowerX.AdminAuthorization.CreateRole(l.ctx, &role); err != nil {
		return nil, err
	}

	return &types.CreateRoleReply{
		RoleCode: role.RoleCode,
	}, nil
}
