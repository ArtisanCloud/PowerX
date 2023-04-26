package schedule

import (
	"PowerX/internal/uc/custom/reservationcenter"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListSchedulesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListSchedulesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListSchedulesLogic {
	return &ListSchedulesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListSchedulesLogic) ListSchedules(req *types.ListSchedulesPageRequest) (resp *types.ListSchedulesPageReply, err error) {

	schedules, err := l.svcCtx.Custom.Schedule.FindAllSchedules(l.ctx, &reservationcenter.FindManySchedulesOption{})

	if err != nil {
		return nil, err
	}

	list := []*types.Schedule{}
	return &types.ListSchedulesPageReply{
		List: list,
	}, nil
}
