package permission

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type SetRoleEmployeesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSetRoleEmployeesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SetRoleEmployeesLogic {
	return &SetRoleEmployeesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *SetRoleEmployeesLogic) SetRoleEmployees(req *types.SetRoleEmployeesRequest) (resp *types.SetRoleEmployeesReply, err error) {
	err = l.svcCtx.PowerX.AdminAuthorization.SetRoleEmployeesByRoleCode(l.ctx, req.EmployeeIds, req.RoleCode)
	if err != nil {
		return nil, err
	}
	return &types.SetRoleEmployeesReply{
		Status: "ok",
	}, nil
}
