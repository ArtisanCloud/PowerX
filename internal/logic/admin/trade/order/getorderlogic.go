package order

import (
	"PowerX/internal/logic/admin/trade/payment"
	"PowerX/internal/model/trade"
	"PowerX/internal/types/errorx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetOrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetOrderLogic {
	return &GetOrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetOrderLogic) GetOrder(req *types.GetOrderRequest) (resp *types.GetOrderReply, err error) {
	mdlOrder, err := l.svcCtx.PowerX.Order.GetOrder(l.ctx, req.OrderId)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetOrderReply{
		Order: TransformOrderToOrderReply(mdlOrder),
	}, nil
}

func TransformOrderToOrderReply(mdlOrder *trade.Order) (orderReply *types.Order) {

	return &types.Order{
		Id:          mdlOrder.Id,
		CustomerId:  mdlOrder.CustomerId,
		PaymentType: mdlOrder.PaymentType,
		Type:        mdlOrder.Type,
		Status:      mdlOrder.Status,
		OrderNumber: mdlOrder.OrderNumber,
		Discount:    mdlOrder.Discount,
		ListPrice:   mdlOrder.ListPrice,
		UnitPrice:   mdlOrder.UnitPrice,
		Comment:     mdlOrder.Comment,
		OrderItems:  TransformOrderItemsToOrderItemsReply(mdlOrder.Items),
		Payments:    payment.TransformPaymentsToReply(mdlOrder.Payments),
		CreatedAt:   mdlOrder.CreatedAt.String(),
	}

}

func TransformOrderItemsToOrderItemsReply(orderItems []*trade.OrderItem) (orderItemsReply []*types.OrderItem) {

	orderItemsReply = []*types.OrderItem{}
	for _, orderItem := range orderItems {
		orderItemReply := TransformOrderItemToOrderItemReply(orderItem)
		orderItemsReply = append(orderItemsReply, orderItemReply)
	}
	return orderItemsReply
}

func TransformOrderItemToOrderItemReply(orderItem *trade.OrderItem) (orderItemReply *types.OrderItem) {
	if orderItem == nil {
		return nil
	}

	return &types.OrderItem{
		Id:        orderItem.Id,
		SkuNo:     orderItem.SkuNo,
		UnitPrice: orderItem.UnitPrice,
		ListPrice: orderItem.ListPrice,
	}
}
