package permission

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/uc/powerx"
	"PowerX/pkg/slicex"
	"context"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRoleEmployeeIdsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetRoleEmployeeIdsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRoleEmployeeIdsLogic {
	return &GetRoleEmployeeIdsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetRoleEmployeeIdsLogic) GetRoleEmployeeIds(req *types.GetRoleEmployeeIdsReqeust) (resp *types.GetRoleEmployeeIdsReply, err error) {
	accounts, _ := l.svcCtx.UC.Auth.Casbin.GetUsersForRole(req.RoleCode)
	if len(accounts) == 0 {
		return &types.GetRoleEmployeeIdsReply{
			EmployeeIds: []int64{},
		}, nil
	}
	employeePage := l.svcCtx.UC.Employee.FindManyEmployees(l.ctx, &powerx.FindManyEmployeeOption{
		Accounts: accounts,
	})
	ids := slicex.SlicePluck(employeePage.List, func(item *powerx.Employee) int64 {
		return item.ID
	})
	return &types.GetRoleEmployeeIdsReply{
		EmployeeIds: ids,
	}, nil
}
