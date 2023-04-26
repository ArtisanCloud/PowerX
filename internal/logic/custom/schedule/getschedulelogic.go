package schedule

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetScheduleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetScheduleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetScheduleLogic {
	return &GetScheduleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetScheduleLogic) GetSchedule(req *types.GetScheduleRequest) (resp *types.GetScheduleReply, err error) {
	// todo: add your logic here and delete this line

	return
}
