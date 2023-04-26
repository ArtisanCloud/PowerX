package schedule

import (
	reservationcenter2 "PowerX/internal/model/custom/reservationcenter"
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

func TransformScheduleToScheduleReply(schedule *reservationcenter2.Schedule) *types.Schedule {
	return &types.Schedule{
		Id:                 schedule.Id,
		StoreId:            schedule.StoreId,
		ApprovalStatus:     schedule.ApprovalStatus,
		Capacity:           schedule.Capacity,
		CopyFromScheduleId: schedule.CopyFromScheduleId,
		Name:               schedule.Name,
		Description:        schedule.Description,
		IsActive:           schedule.IsActive,
		Status:             schedule.Status,
		StartTime:          schedule.StartTime.String(),
		EndTime:            schedule.EndTime.String(),
		CreatedAt:          schedule.CreatedAt.String(),
	}
}
