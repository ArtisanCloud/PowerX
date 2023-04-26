package schedule

import (
	reservationcenter2 "PowerX/internal/model/custom/reservationcenter"
	"PowerX/internal/uc/custom/reservationcenter"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListSchedulesPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListSchedulesPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListSchedulesPageLogic {
	return &ListSchedulesPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListSchedulesPageLogic) ListSchedulesPage(req *types.ListSchedulesPageRequest) (resp *types.ListSchedulesPageReply, err error) {
	schedules, err := l.svcCtx.Custom.Schedule.FindManySchedules(l.ctx, &reservationcenter.FindManySchedulesOption{
		StoreId: req.StoreId,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})

	if err != nil {
		return nil, err
	}

	return &types.ListSchedulesPageReply{
		List:      TransformSchedulesToSchedulesReply(schedules.List),
		PageIndex: schedules.PageIndex,
		PageSize:  schedules.PageSize,
		Total:     schedules.Total,
	}, nil

}

func TransformSchedulesToSchedulesReply(schedule []*reservationcenter2.Schedule) []*types.Schedule {

	list := make([]*types.Schedule, 0, len(schedule))
	for _, item := range schedule {
		list = append(list, TransformScheduleToScheduleReply(item))
	}

	return list
}
