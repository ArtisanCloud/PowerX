package permission

import (
	"PowerX/internal/uc/powerx"
	"context"

	"PowerX/internal/model"
	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchRoleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchRoleLogic {
	return &PatchRoleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchRoleLogic) PatchRole(req *types.PatchRoleReqeust) (resp *types.PatchRoleReply, err error) {
	var adminAPI []*powerx.AdminAPI
	for _, id := range req.APIIds {
		adminAPI = append(adminAPI, &powerx.AdminAPI{
			CommonModel: model.CommonModel{
				Id: id,
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

	l.svcCtx.PowerX.AdminAuthorization.PatchRoleByRoleCode(l.ctx, &role, req.RoleCode)

	return &types.PatchRoleReply{
		AdminRole: &types.AdminRole{
			RoleCode:   role.RoleCode,
			Name:       role.Name,
			Desc:       role.Desc,
			IsReserved: role.IsReserved,
		},
	}, nil
}
