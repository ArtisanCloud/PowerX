package common

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetEmployeeQueryOptionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetEmployeeQueryOptionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetEmployeeQueryOptionsLogic {
	return &GetEmployeeQueryOptionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetEmployeeQueryOptionsLogic) GetEmployeeQueryOptions() (resp *types.GetEmployeeQueryOptionsReply, err error) {
	resp = &types.GetEmployeeQueryOptionsReply{}

	resp.Positions = l.svcCtx.PowerX.Organization.FindAllPositions(l.ctx)

	roles := l.svcCtx.PowerX.Auth.FindAllRoles(l.ctx)
	for _, role := range roles {
		resp.Roles = append(resp.Roles, types.EmployeeQueryRoleOption{
			RoleCode: role.RoleCode,
			RoleName: role.Name,
		})
	}

	deps := l.svcCtx.PowerX.Organization.FindAllDepartments(l.ctx)
	for _, dep := range deps {
		resp.Departments = append(resp.Departments, types.EmployeeQueryDepartmentOption{
			DepartmentId:   dep.ID,
			DepartmentName: dep.Name,
		})
	}
	return
}
