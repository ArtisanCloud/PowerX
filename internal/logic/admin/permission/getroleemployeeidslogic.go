package permission

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

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
	// todo: add your logic here and delete this line

	return
}
