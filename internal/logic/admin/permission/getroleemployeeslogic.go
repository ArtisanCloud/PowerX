package permission

import (
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
	// todo: add your logic here and delete this line

	return
}
