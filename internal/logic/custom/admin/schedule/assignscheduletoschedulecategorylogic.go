package schedule

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type AssignScheduleToScheduleCategoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignScheduleToScheduleCategoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignScheduleToScheduleCategoryLogic {
	return &AssignScheduleToScheduleCategoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AssignScheduleToScheduleCategoryLogic) AssignScheduleToScheduleCategory(req *types.AssignScheduleToScheduleCategoryRequest) (resp *types.AssignScheduleToScheduleCategoryReply, err error) {
	// todo: add your logic here and delete this line

	return
}
