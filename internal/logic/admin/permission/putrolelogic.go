package permission

import (
	"PowerX/internal/uc/powerx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutRoleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutRoleLogic {
	return &PutRoleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutRoleLogic) PutRole(req *types.PutRoleReqeust) (resp *types.PutRoleReply, err error) {
	var adminAPI []*powerx.AdminAPI
	for _, id := range req.APIIds {
		adminAPI = append(adminAPI, &powerx.AdminAPI{
			Model: types.Model{
				ID: id,
			},
		})
	}

	var menuNames []*powerx.AdminRoleMenuName
	for _, menuName := range req.MenuNames {
		menuNames = append(menuNames, &powerx.AdminRoleMenuName{
			MenuName: menuName,
		})
	}

	role := powerx.AdminRole{
		RoleCode:   req.RoleCode,
		Name:       req.Name,
		Desc:       req.Desc,
		IsReserved: false,
		AdminAPI:   adminAPI,
		MenuNames:  menuNames,
	}

	l.svcCtx.PowerX.Auth.PatchRoleByRoleCode(l.ctx, &role, req.RoleCode)

	return &types.PutRoleReply{
		AdminRole: &types.AdminRole{
			RoleCode:   role.RoleCode,
			Name:       role.Name,
			Desc:       role.Desc,
			IsReserved: role.IsReserved,
		},
	}, nil
}
