package permission

import (
	"PowerX/internal/uc/powerx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRoleEmployeesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetRoleEmployeesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRoleEmployeesLogic {
	return &GetRoleEmployeesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetRoleEmployeesLogic) GetRoleEmployees(req *types.GetRoleEmployeesReqeust) (resp *types.GetRoleEmployeesReply, err error) {
	accounts, _ := l.svcCtx.PowerX.Auth.Casbin.GetUsersForRole(req.RoleCode)
	employeePage := l.svcCtx.PowerX.Organization.FindManyEmployeesPage(l.ctx, &powerx.FindManyEmployeesOption{
		Accounts:  accounts,
		PageIndex: req.PageIndex,
		PageSize:  req.PageSize,
	})

	resp = &types.GetRoleEmployeesReply{
		PageIndex: employeePage.PageIndex,
		PageSize:  employeePage.PageSize,
		Total:     employeePage.Total,
	}
	return
}
