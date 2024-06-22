package customer

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignCustomerToUserLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignCustomerToUserLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignCustomerToUserLogic {
	return &AssignCustomerToUserLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignCustomerToUserLogic) AssignCustomerToUser(req *types.AssignCustomerToUserRequest) (resp *types.AssignCustomerToUserReply, err error) {
	// todo: add your logic here and delete this line

	return
}
