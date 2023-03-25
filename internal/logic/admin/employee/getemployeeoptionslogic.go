package employee

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetEmployeeOptionsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetEmployeeOptionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetEmployeeOptionsLogic {
	return &GetEmployeeOptionsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetEmployeeOptionsLogic) GetEmployeeOptions() (resp *types.GetEmployeeOptionsReply, err error) {
	resp = &types.GetEmployeeOptionsReply{}

	resp.Positions = l.svcCtx.PowerX.Organization.FindAllPositions(l.ctx)

	roles := l.svcCtx.PowerX.Auth.FindAllRoles(l.ctx)
	for _, role := range roles {
		resp.Roles = append(resp.Roles, types.RoleOption{
			RoleCode: role.RoleCode,
			RoleName: role.Name,
		})
	}

	deps := l.svcCtx.PowerX.Organization.FindAllDepartments(l.ctx)
	for _, dep := range deps {
		resp.Departments = append(resp.Departments, types.DepartmentOption{
			DepartmentId:   dep.ID,
			DepartmentName: dep.Name,
		})
	}
	return
}
