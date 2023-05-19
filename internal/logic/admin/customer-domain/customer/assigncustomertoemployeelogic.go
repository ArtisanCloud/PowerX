package customer

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignCustomerToEmployeeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignCustomerToEmployeeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignCustomerToEmployeeLogic {
	return &AssignCustomerToEmployeeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignCustomerToEmployeeLogic) AssignCustomerToEmployee(req *types.AssignCustomerToEmployeeRequest) (resp *types.AssignCustomerToEmployeeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
