package order

import (
	"PowerX/internal/model/trade"
	tradeUC "PowerX/internal/uc/powerx/trade"
	"PowerX/pkg/datetime/carbonx"
	"context"
	"github.com/golang-module/carbon/v2"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListOrdersPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListOrdersPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListOrdersPageLogic {
	return &ListOrdersPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListOrdersPageLogic) ListOrdersPage(req *types.ListOrdersPageRequest) (resp *types.ListOrdersPageReply, err error) {
	startAt := carbon.ParseByFormat(req.StartAt, carbonx.DateFormat)
	endAt := carbon.ParseByFormat(req.EndAt, carbonx.DateFormat)
	if !startAt.IsZero() && endAt.IsZero() {
		endAt = startAt.AddDays(30)
	} else if startAt.IsZero() && !endAt.IsZero() {
		startAt = endAt.AddDays(-30)
	}
	//fmt.Dump(startAt.String(), endAt.String())

	page, err := l.svcCtx.PowerX.Order.FindManyOrders(l.ctx, &tradeUC.FindManyOrdersOption{
		StartAt:  startAt.ToStdTime(),
		EndAt:    endAt.ToStdTime(),
		LikeName: req.Name,
		Status:   req.StatusIds,
		Type:     req.TypeIds,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := TransformOrdersToReply(page.List)
	return &types.ListOrdersPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil
}

func TransformOrdersToReply(orders []*trade.Order) []*types.Order {
	ordersReply := []*types.Order{}
	for _, order := range orders {
		orderReply := TransformOrderToOrderReply(order)
		ordersReply = append(ordersReply, orderReply)

	}
	return ordersReply
}
