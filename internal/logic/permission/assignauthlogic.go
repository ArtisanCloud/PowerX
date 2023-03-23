package permission

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/uc/powerx"
	"PowerX/pkg/slicex"
	"context"
	"github.com/pkg/errors"
	"github.com/zeromicro/go-zero/core/logx"
)

type AssignAuthLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignAuthLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignAuthLogic {
	return &AssignAuthLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignAuthLogic) AssignAuth(req *types.AssignAuthRequest) error {
	if req.RoleAssignRes != nil {
		acts := l.svcCtx.UC.Auth.FindManyRestActsByIds(l.ctx, req.RoleAssignRes.ActIds)
		for _, code := range req.RoleAssignRes.RoleCodes {
			for _, act := range acts {
				_, err := l.svcCtx.UC.Auth.Casbin.AddPolicy(code, powerx.FormatAuthRestObj(act.ResCode, act.Version, act.RestPath), act.Action)
				if err != nil {
					panic(errors.Wrap(err, "add policy failed"))
				}
			}
		}

		//todo replace
		return nil
	}

	if req.UserAssignRole != nil {
		userPage := l.svcCtx.UC.Employee.FindManyEmployees(l.ctx, &powerx.FindManyEmployeeOption{
			Ids: req.UserAssignRole.UserIds,
		})
		if len(userPage.List) == 0 {
			return nil
		}
		accounts := slicex.SlicePluck(userPage.List, func(item *powerx.Employee) string {
			return item.Account
		})

		if req.UserAssignRole.IsReplace {
			for _, account := range accounts {
				l.svcCtx.UC.Auth.UpdateRolesForEmployee(l.ctx, account, req.UserAssignRole.RoleCodes)
			}
		} else {
			for _, account := range accounts {
				l.svcCtx.UC.Auth.AddRolesForEmployee(l.ctx, account, req.UserAssignRole.RoleCodes)
			}
		}
	}

	if req.RoleAssignUsers != nil {
		// todo validate roleCode
		_, _ = l.svcCtx.UC.Auth.Casbin.RemoveFilteredGroupingPolicy(1, req.RoleAssignUsers.RoleCode)
		if len(req.RoleAssignUsers.UserIds) == 0 {
			return nil
		}
		userPage := l.svcCtx.UC.Employee.FindManyEmployees(l.ctx, &powerx.FindManyEmployeeOption{
			Ids: req.RoleAssignUsers.UserIds,
		})
		if len(userPage.List) == 0 {
			return nil
		}
		accounts := slicex.SlicePluck(userPage.List, func(item *powerx.Employee) string {
			return item.Account
		})
		if len(accounts) > 0 {
			for _, account := range accounts {
				_, _ = l.svcCtx.UC.Auth.Casbin.AddRoleForUser(account, req.RoleAssignUsers.RoleCode)
			}
		}
	}

	return nil
}
