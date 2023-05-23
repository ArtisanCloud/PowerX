package order

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
	product2 "PowerX/internal/model/product"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/customerdomain"
	"PowerX/internal/uc/powerx/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateOrderByProductsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateOrderByProductsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateOrderByProductsLogic {
	return &CreateOrderByProductsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateOrderByProductsLogic) CreateOrderByProducts(req *types.CreateOrderByProductsRequest) (resp *types.CreateOrderByProductsReply, err error) {
	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	if req.PriceBookId <= 0 {
		standardBook, err := l.svcCtx.PowerX.PriceBook.GetStandardPriceBook(l.ctx)
		if err != nil {
			return nil, errorx.WithCause(errorx.ErrNotFoundObject, "未找到标准价格手册")
		}
		req.PriceBookId = standardBook.Id
	}

	entries := []*product2.PriceBookEntry{}
	if len(req.SkuIds) > 0 {
		page := l.svcCtx.PowerX.PriceBookEntry.FindManyPriceBookEntries(l.ctx, &product.FindPriceBookEntryOption{
			PriceBookId: req.PriceBookId,
			SkuIds:      req.SkuIds,
		})
		entries = page.List

	} else if len(req.ProductIds) > 0 {
		//如果搜索ProductId，那么就要排除掉SKU的选项
		page := l.svcCtx.PowerX.PriceBookEntry.FindManyPriceBookEntries(l.ctx, &product.FindPriceBookEntryOption{
			ProductIds: req.ProductIds,
		})
		entries = page.List

	} else {
		return nil, errorx.WithCause(errorx.ErrNotFoundObject, "请求相应的商品信息有误")
	}

	if len(entries) <= 0 {
		return nil, errorx.WithCause(errorx.ErrNotFoundObject, "无法找到相应的商品信息")
	}

	if len(req.Quantities) != len(entries) {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "购买数量和购买产品无法匹配")
	}

	// 查找收货地址
	shippingAddress, err := l.svcCtx.PowerX.ShippingAddress.GetShippingAddress(l.ctx, req.ShippingAddressId)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrRecordNotFound, "未找到地址记录")
	}

	// 创建订单
	order, err := l.svcCtx.PowerX.Order.CreateOrderByPriceBookEntries(
		l.ctx, authCustomer, entries,
		req.Quantities, shippingAddress, req.Comment,
	)
	if err != nil {
		return nil, errorx.WithCause(errorx.ErrCreateObject, err.Error())
	}

	return &types.CreateOrderByProductsReply{
		OrderId: order.Id,
	}, err
}
