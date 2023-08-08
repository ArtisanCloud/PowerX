package order

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
	"PowerX/internal/model/trade"
	"PowerX/internal/uc/powerx/customerdomain"
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
	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	page, err := l.svcCtx.PowerX.Order.FindManyOrders(l.ctx, &tradeUC.FindManyOrdersOption{
		CustomerId: authCustomer.Id,
		Status:     req.OrderStatus,
		Type:       req.OrderType,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := TransformOrdersToReplyForMP(page.List)
	return &types.ListOrdersPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil

}

func TransformOrdersToReplyForMP(orders []*trade.Order) (ordersReply []*types.Order) {
	ordersReply = []*types.Order{}
	for _, order := range orders {
		orderReply := TransformOrderToReplyForMP(order)
		ordersReply = append(ordersReply, orderReply)

	}
	return ordersReply
}
