package order

import (
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/trade"
	fmt "PowerX/pkg/printx"
	"context"
	"github.com/golang-module/carbon/v2"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ExportOrdersLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewExportOrdersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ExportOrdersLogic {
	return &ExportOrdersLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ExportOrdersLogic) ExportOrders(req *types.ExportOrdersRequest) (resp *types.ExportOrdersReply, err error) {

	startAt := carbon.Parse(req.StartAt)
	endAt := carbon.Parse(req.EndAt)

	orders, err := l.svcCtx.PowerX.Order.FindAllOrders(l.ctx, &trade.FindManyOrdersOption{
		StartAt: startAt.ToStdTime(),
		EndAt:   endAt.ToStdTime(),
	})
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrNotFoundObject, err.Error())
	}
	fmt.Dump(orders)
	//forgi

	return
}
