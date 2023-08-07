package employee

import (
	"PowerX/internal/model/option"
	"PowerX/internal/model/origanzation"
	"PowerX/internal/model/permission"
	"PowerX/pkg/slicex"
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
	opt := option.FindManyEmployeesOption{
		Ids:             req.Ids,
		LikeName:        req.LikeName,
		LikeEmail:       req.LikeEmail,
		DepIds:          req.DepIds,
		PositionIDs:     req.PositionIds,
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
			opt.Statuses = append(opt.Statuses, origanzation.EmployeeStatusEnabled)
		} else {
			opt.Statuses = append(opt.Statuses, origanzation.EmployeeStatusDisabled)
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
		vo := types.Employee{
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
			JobTitle:      employee.JobTitle,
			IsEnabled:     employee.Status == origanzation.EmployeeStatusEnabled,
			CreatedAt:     employee.CreatedAt.Format(time.RFC3339),
		}
		if employee.Position != nil {
			codes := slicex.SlicePluck(employee.Position.Roles, func(item *permission.AdminRole) string {
				return item.RoleCode
			})
			vo.Position = &types.Position{
				Id:        employee.Position.Id,
				Name:      employee.Position.Name,
				Desc:      employee.Position.Desc,
				Level:     employee.Position.Level,
				RoleCodes: codes,
			}
			vo.PositionId = employee.Position.Id
		}

		vos = append(vos, vo)
	}

	return &types.ListEmployeesReply{
		List:      vos,
		PageIndex: employeePage.PageIndex,
		PageSize:  employeePage.PageSize,
		Total:     employeePage.Total,
	}, nil
}
