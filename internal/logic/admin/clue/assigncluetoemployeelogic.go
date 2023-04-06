package clue

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignClueToEmployeeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignClueToEmployeeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignClueToEmployeeLogic {
	return &AssignClueToEmployeeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignClueToEmployeeLogic) AssignClueToEmployee(req *types.AssignClueToEmployeeRequest) (resp *types.AssignClueToEmployeeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
