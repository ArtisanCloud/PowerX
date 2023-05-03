package schedule

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchScheduleLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchScheduleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchScheduleLogic {
	return &PatchScheduleLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchScheduleLogic) PatchSchedule(req *types.PatchScheduleRequest) (resp *types.PatchScheduleReply, err error) {
	// todo: add your logic here and delete this line

	return
}
