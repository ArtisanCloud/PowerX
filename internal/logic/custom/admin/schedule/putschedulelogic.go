package schedule

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutScheduleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutScheduleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutScheduleLogic {
	return &PutScheduleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutScheduleLogic) PutSchedule(req *types.PutScheduleRequest) (resp *types.PutScheduleReply, err error) {
	// todo: add your logic here and delete this line

	return
}
