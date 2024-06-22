package common

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetUserQueryOptionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetUserQueryOptionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetUserQueryOptionsLogic {
	return &GetUserQueryOptionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetUserQueryOptionsLogic) GetUserQueryOptions() (resp *types.GetUserQueryOptionsReply, err error) {
	resp = &types.GetUserQueryOptionsReply{}

	roles := l.svcCtx.PowerX.AdminAuthorization.FindAllRoles(l.ctx)
	for _, role := range roles {
		resp.Roles = append(resp.Roles, types.UserQueryRoleOption{
			RoleCode: role.RoleCode,
			RoleName: role.Name,
		})
	}

	deps := l.svcCtx.PowerX.Organization.FindAllDepartments(l.ctx)
	for _, dep := range deps {
		resp.Departments = append(resp.Departments, types.UserQueryDepartmentOption{
			DepartmentId:   dep.Id,
			DepartmentName: dep.Name,
		})
	}
	return
}
