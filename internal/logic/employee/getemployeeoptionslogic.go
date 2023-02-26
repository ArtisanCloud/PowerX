package employee

import (
	"PowerX/internal/uc"
	"PowerX/pkg/slicex"
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

type GetEmployeeOptionScope string

const (
	GetEmployeeOptionScopePosition   = "position"
	GetEmployeeOptionScopeRole       = "role"
	GetEmployeeOptionScopeDepartment = "department"
)

func (l *GetEmployeeOptionsLogic) GetEmployeeOptions(req *types.GetEmployeeOptionsRequest) (resp *types.GetEmployeeOptionsReply, err error) {
	if len(req.Scopes) == 0 {
		req.Scopes = append(req.Scopes, GetEmployeeOptionScopePosition, GetEmployeeOptionScopeRole, GetEmployeeOptionScopeDepartment)
	}

	resp = &types.GetEmployeeOptionsReply{}
	if slicex.Contains(req.Scopes, GetEmployeeOptionScopePosition) {
		resp.Positions = l.svcCtx.UC.Employee.GetAllPositions(l.ctx)
	}
	if slicex.Contains(req.Scopes, GetEmployeeOptionScopeRole) {
		roles := l.svcCtx.UC.Auth.FindManyAuthRoles(l.ctx)
		var vos []types.SimpleRole
		for _, role := range roles {
			vos = append(vos, types.SimpleRole{
				RoleCode: role.RoleCode,
				RoleName: role.Name,
			})
		}
		resp.Roles = vos
	}
	if slicex.Contains(req.Scopes, GetEmployeeOptionScopeDepartment) {
		deps := l.svcCtx.UC.Department.FindManyDepartments(l.ctx, &uc.FindManyDepartmentsOption{})
		var vos []types.SimpleDepartment
		for _, department := range deps.List {
			vos = append(vos, types.SimpleDepartment{
				Id:      department.ID,
				DepName: department.Name,
			})
		}
		resp.Departments = vos
	}

	return
}
