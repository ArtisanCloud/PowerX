package order

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
	"PowerX/internal/model/trade"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/customerdomain"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CancelOrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCancelOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CancelOrderLogic {
	return &CancelOrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CancelOrderLogic) CancelOrder(req *types.CancelOrderRequest) (resp *types.CancelOrderReply, err error) {
	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	// 找出相应的Cart Items
	order, err := l.svcCtx.PowerX.Order.GetOrder(l.ctx, req.OrderId)
	if l.svcCtx.PowerX.Order.CanOrderCancel(l.ctx, order) {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "订单状态不能被取消")
	}

	if order.CustomerId != authCustomer.Id {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "无权取消该订单")
	}
	orderStatusId := l.svcCtx.PowerX.Order.GetOrderStatusId(l.ctx, trade.OrderStatusCancelled)
	order.Status = orderStatusId
	l.svcCtx.PowerX.Order.PatchOrder(l.ctx, req.OrderId, order)

	return &types.CancelOrderReply{
		OrderId: order.Id,
	}, nil
}
