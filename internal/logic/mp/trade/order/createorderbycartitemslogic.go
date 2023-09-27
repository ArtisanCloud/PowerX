package order

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/crm/customerdomain"
	"PowerX/internal/uc/powerx/crm/trade"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateOrderByCartItemsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateOrderByCartItemsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateOrderByCartItemsLogic {
	return &CreateOrderByCartItemsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateOrderByCartItemsLogic) CreateOrderByCartItems(req *types.CreateOrderByCartItemsRequest) (resp *types.CreateOrderByCartItemsReply, err error) {
	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	// 找出相应的Cart Items
	cartItems, err := l.svcCtx.PowerX.Cart.FindAllCartItems(l.ctx, &trade.FindManyCartItemsOption{
		CustomerId: authCustomer.Id,
		Ids:        req.CartItemIds,
	})
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrRecordNotFound, err.Error())
	}
	if len(cartItems) <= 0 {
		return nil, errorx.WithCause(errorx.ErrRecordNotFound, "购物车商品空，请重新添加商品")
	}

	// 查找收货地址
	shippingAddress, err := l.svcCtx.PowerX.ShippingAddress.GetShippingAddress(l.ctx, req.ShippingAddressId)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrRecordNotFound, err.Error())
	}

	// 创建订单
	order, cart, err := l.svcCtx.PowerX.Order.CreateOrderByCartItems(l.ctx, authCustomer, cartItems, shippingAddress, req.Comment)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrCreateObject, err.Error())
	}

	return &types.CreateOrderByCartItemsReply{
		OrderId:       order.Id,
		CartId:        cart.Id,
		PaymentAmount: order.UnitPrice,
	}, err

}
