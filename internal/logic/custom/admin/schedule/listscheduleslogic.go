package schedule

import (
	"PowerX/internal/svc"
	"PowerX/internal/types"
	"PowerX/internal/uc/custom/reservationcenter"
	"context"
	"github.com/golang-module/carbon/v2"
	"github.com/pkg/errors"
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

func (l *ListSchedulesLogic) ListSchedules(req *types.ListSchedulesRequest) (resp *types.ListSchedulesReply, err error) {

	now := carbon.Now()
	var cDate carbon.Carbon
	if req.CurrentDate != "" {
		cDate = carbon.Parse(req.CurrentDate)
		if cDate.IsInvalid() {
			return nil, errors.New("查询的当前时间格式无效")
		}
	} else {
		cDate = now
	}

	schedules, err := l.svcCtx.Custom.Schedule.FindAllSchedules(l.ctx, &reservationcenter.FindManySchedulesOption{
		CurrentDate: cDate.ToStdTime(),
		StoreId:     req.StoreId,
	})

	if err != nil {
		return nil, err
	}

	return &types.ListSchedulesReply{
		List: TransformSchedulesToSchedulesReply(schedules),
	}, nil
}
