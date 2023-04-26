package schedule

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteScheduleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteScheduleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteScheduleLogic {
	return &DeleteScheduleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteScheduleLogic) DeleteSchedule(req *types.DeleteScheduleRequest) (resp *types.DeleteScheduleReply, err error) {
	// todo: add your logic here and delete this line

	return
}
