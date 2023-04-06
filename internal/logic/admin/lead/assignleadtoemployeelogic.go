package lead

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignLeadToEmployeeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignLeadToEmployeeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignLeadToEmployeeLogic {
	return &AssignLeadToEmployeeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignLeadToEmployeeLogic) AssignLeadToEmployee(req *types.AssignLeadToEmployeeRequest) (resp *types.AssignLeadToEmployeeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
