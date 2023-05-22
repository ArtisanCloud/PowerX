package cart

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

type AddToCartLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAddToCartLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AddToCartLogic {
	return &AddToCartLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *AddToCartLogic) AddToCart(req *types.AddToCartRequest) (resp *types.AddToCartReply, err error) {

	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	cartItem := TransformCartItemRequestToCartItemToMP(req, authCustomer)

	cartItem, err = l.svcCtx.PowerX.Cart.AddItemToCart(l.ctx, cartItem)

	if err != nil {
		return nil, errorx.WithCause(errorx.ErrCreateObject, err.Error())
	}

	return &types.AddToCartReply{
		TransformCartItemToCartItemReplyToMP(cartItem),
	}, nil
}

func TransformCartItemRequestToCartItemToMP(req *types.AddToCartRequest, customer *customerdomain2.Customer) *trade.CartItem {

	item := &trade.CartItem{
		CustomerId:     customer.Id,
		ProductId:      req.ProductId,
		SkuId:          req.SkuId,
		ProductName:    req.ProductName,
		ListPrice:      req.ListPrice,
		UnitPrice:      req.UnitPrice,
		Discount:       req.Discount,
		Quantity:       req.Quantity,
		Specifications: req.Specifications,
		ImageURL:       req.ImageURL,
	}

	return item
}

func TransformCartItemToCartItemReplyToMP(item *trade.CartItem) *types.CartItem {

	return &types.CartItem{
		Id:             item.Id,
		CustomerId:     item.CustomerId,
		CartId:         item.CartId,
		ProductId:      item.ProductId,
		SkuId:          item.SkuId,
		ProductName:    item.ProductName,
		ListPrice:      item.ListPrice,
		UnitPrice:      item.UnitPrice,
		Discount:       item.Discount,
		Quantity:       item.Quantity,
		Specifications: item.Specifications,
		ImageURL:       item.ImageURL,
	}
}
