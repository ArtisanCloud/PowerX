package permission

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/uc"
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
		for _, code := range req.RoleAssignRes.RoleCodes {
			for _, act := range req.RoleAssignRes.Acts {
				_, err := l.svcCtx.UC.Auth.Casbin.AddPolicy(code, act.ResCode, act.Action)
				if err != nil {
					panic(errors.Wrap(err, "add policy failed"))
				}
			}
		}

		//todo replace
		return nil
	}

	if req.UserAssignRes != nil {
		userPage := l.svcCtx.UC.Employee.FindManyEmployees(l.ctx, &uc.FindEmployeeOption{
			Ids: req.UserAssignRes.UserIds,
		})
		if len(userPage.List) == 0 {
			return nil
		}
		accounts := slicex.SlicePluck(userPage.List, func(item *uc.Employee) string {
			return item.Account
		})

		if req.UserAssignRes.IsReplace {
			for _, account := range accounts {
				l.svcCtx.UC.Auth.UpdateRolesForEmployee(l.ctx, account, req.UserAssignRes.RoleCodes)
			}
		} else {
			for _, account := range accounts {
				l.svcCtx.UC.Auth.AddRolesForEmployee(l.ctx, account, req.UserAssignRes.RoleCodes)
			}
		}
	}
	return nil
}
