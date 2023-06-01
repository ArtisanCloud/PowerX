package order

import (
	"PowerX/internal/model/trade"
	tradeUC "PowerX/internal/uc/powerx/trade"
	"context"

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
	page, err := l.svcCtx.PowerX.Order.FindManyOrders(l.ctx, &tradeUC.FindManyOrdersOption{
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := TransformOrdersToOrdersReply(page.List)
	return &types.ListOrdersPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil
}

func TransformOrdersToOrdersReply(orders []*trade.Order) []*types.Order {
	ordersReply := []*types.Order{}
	for _, order := range orders {
		orderReply := TransformOrderToOrderReply(order)
		ordersReply = append(ordersReply, orderReply)

	}
	return ordersReply
}
