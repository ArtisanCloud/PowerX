package order

import (
	"PowerX/internal/logic/admin/crm/trade/payment"
	"PowerX/internal/logic/admin/mediaresource"
	"PowerX/internal/model/crm/trade"
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
		Order: TransformOrderToReply(mdlOrder),
	}, nil
}

func TransformOrderToReply(mdlOrder *trade.Order) (orderReply *types.Order) {

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
		Logistics:   TransformLogisticsToReply(mdlOrder.Logistics),
		CreatedAt:   mdlOrder.CreatedAt.String(),
	}

}

func TransformOrderItemsToOrderItemsReply(orderItems []*trade.OrderItem) (orderItemsReply []*types.OrderItem) {

	orderItemsReply = []*types.OrderItem{}
	for _, orderItem := range orderItems {
		orderItemReply := TransformOrderItemToReply(orderItem)
		orderItemsReply = append(orderItemsReply, orderItemReply)
	}
	return orderItemsReply
}

func TransformOrderItemToReply(orderItem *trade.OrderItem) (orderItemReply *types.OrderItem) {
	if orderItem == nil {
		return nil
	}

	return &types.OrderItem{
		Id:          orderItem.Id,
		SkuNo:       orderItem.SkuNo,
		ProductName: orderItem.ProductName,
		UnitPrice:   orderItem.UnitPrice,
		ListPrice:   orderItem.ListPrice,
		Quantity:    orderItem.Quantity,
		CoverImage:  mediaresource.TransformMediaResourceToReply(orderItem.CoverImage),
	}
}

func TransformLogisticsToReply(logistics *trade.Logistics) *types.Logistics {
	if logistics == nil {
		return nil
	}

	return &types.Logistics{
		OrderId:               logistics.OrderId,
		Status:                string(logistics.Status),
		TrackingCode:          logistics.TrackingCode,
		Carrier:               logistics.Carrier,
		EstimatedDeliveryDate: logistics.EstimatedDeliveryDate.String(),
		ActualDeliveryDate:    logistics.ActualDeliveryDate.String(),
	}

}
