package employee

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/uc/powerx"
	"context"
	"github.com/zeromicro/go-zero/core/logx"
	"time"
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
	opt := powerx.FindEmployeeOption{
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
			as, _ := l.svcCtx.UC.Auth.Casbin.GetUsersForRole(code)
			accounts = append(accounts, as...)
		}
		// 涉及角色查询, root账户会出现在所有角色筛选中
		accounts = append(accounts, "root")
		opt.Accounts = accounts
	}
	if req.IsEnabled != nil {
		if *req.IsEnabled {
			opt.Statuses = append(opt.Statuses, powerx.EmployeeStatusEnable)
		} else {
			opt.Statuses = append(opt.Statuses, powerx.EmployeeStatusDisable)
		}
	}

	employeePage := l.svcCtx.UC.Employee.FindManyEmployees(l.ctx, &opt)

	var employeeIds []int64
	var depIds []int64
	for _, employee := range employeePage.List {
		employeeIds = append(employeeIds, employee.ID)
		depIds = append(depIds, employee.DepartmentIds...)
	}

	// build vo
	var vos []types.Employee
	for _, employee := range employeePage.List {
		roles, _ := l.svcCtx.UC.Auth.Casbin.GetRolesForUser(employee.Account)

		isEnabled := *employee.Status > 0
		gender := int8(0)
		if employee.Gender != nil {
			gender = int8(*employee.Gender)
		}
		vos = append(vos, types.Employee{
			Id:            employee.ID,
			Account:       employee.Account,
			Name:          employee.Name,
			Email:         employee.Email,
			MobilePhone:   employee.MobilePhone,
			Gender:        gender,
			NickName:      employee.NickName,
			Desc:          employee.Desc,
			Avatar:        employee.Avatar,
			ExternalEmail: employee.ExternalEmail,
			DepIds:        employee.DepartmentIds,
			Roles:         roles,
			Position:      employee.Position,
			JobTitle:      employee.JobTitle,
			IsEnabled:     isEnabled,
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
