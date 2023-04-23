package employee

import (
	"PowerX/internal/model/scrm/organization"
	"PowerX/internal/uc/powerx"
	"context"
	"time"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListEmployeesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListEmployeesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListEmployeesLogic {
	return &ListEmployeesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListEmployeesLogic) ListEmployees(req *types.ListEmployeesRequest) (resp *types.ListEmployeesReply, err error) {
	opt := powerx.FindManyEmployeesOption{
		Ids:             req.Ids,
		LikeName:        req.LikeName,
		LikeEmail:       req.LikeEmail,
		DepIds:          req.DepIds,
		Positions:       req.Positions,
		LikePhoneNumber: req.LikePhoneNumber,
		PageIndex:       req.PageIndex,
		PageSize:        req.PageSize,
	}

	if len(req.RoleCodes) > 0 {
		// bind roles opt, todo improve performance or remove it
		var accounts []string
		for _, code := range req.RoleCodes {
			as, _ := l.svcCtx.PowerX.AdminAuthorization.Casbin.GetUsersForRole(code)
			accounts = append(accounts, as...)
		}
		// 涉及角色查询, root账户会出现在所有角色筛选中
		accounts = append(accounts, "root")
		opt.Accounts = accounts
	}
	if req.IsEnabled != nil {
		if *req.IsEnabled {
			opt.Statuses = append(opt.Statuses, organization.EmployeeStatusEnabled)
		} else {
			opt.Statuses = append(opt.Statuses, organization.EmployeeStatusDisabled)
		}
	}

	employeePage := l.svcCtx.PowerX.Organization.FindManyEmployeesPage(l.ctx, &opt)

	// build vo
	var vos []types.Employee
	for _, employee := range employeePage.List {
		roles, _ := l.svcCtx.PowerX.AdminAuthorization.Casbin.GetRolesForUser(employee.Account)
		var dep *types.EmployeeDepartment
		if employee.Department != nil {
			dep = &types.EmployeeDepartment{
				DepId:   employee.Department.Id,
				DepName: employee.Department.Name,
			}
		}
		vos = append(vos, types.Employee{
			Id:            employee.Id,
			Account:       employee.Account,
			Name:          employee.Name,
			Email:         employee.Email,
			MobilePhone:   employee.MobilePhone,
			Gender:        employee.Gender,
			NickName:      employee.NickName,
			Desc:          employee.Desc,
			Avatar:        employee.Avatar,
			ExternalEmail: employee.ExternalEmail,
			Department:    dep,
			Roles:         roles,
			Position:      employee.Position,
			JobTitle:      employee.JobTitle,
			IsEnabled:     employee.Status == organization.EmployeeStatusEnabled,
			CreatedAt:     employee.CreatedAt.Format(time.RFC3339),
		})
	}

	return &types.ListEmployeesReply{
		List:      vos,
		PageIndex: employeePage.PageIndex,
		PageSize:  employeePage.PageSize,
		Total:     employeePage.Total,
	}, nil
}
