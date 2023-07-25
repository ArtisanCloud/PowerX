package permission

import (
	"PowerX/internal/model/permission"
	"context"

	"PowerX/internal/model"
	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SetRolePermissionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSetRolePermissionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetRolePermissionsLogic {
	return &SetRolePermissionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SetRolePermissionsLogic) SetRolePermissions(req *types.SetRolePermissionsRequest) (resp *types.SetRolePermissionsReply, err error) {
	var role permission.AdminRole

	var api []*permission.AdminAPI
	for _, id := range req.APIIds {
		api = append(api, &permission.AdminAPI{
			CommonModel: model.CommonModel{
				Id: id,
			},
		})
	}

	role.AdminAPI = api

	l.svcCtx.PowerX.AdminAuthorization.PatchRoleByRoleCode(l.ctx, &role, req.RoleCode)

	return &types.SetRolePermissionsReply{
		Status: "ok",
	}, nil
}
