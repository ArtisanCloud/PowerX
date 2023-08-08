package cart

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

type ListCartItemsPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListCartItemsPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListCartItemsPageLogic {
	return &ListCartItemsPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListCartItemsPageLogic) ListCartItemsPage(req *types.ListCartItemsPageRequest) (resp *types.ListCartItemsPageReply, err error) {

	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	page, err := l.svcCtx.PowerX.Cart.FindManyCartItems(l.ctx, &tradeUC.FindManyCartItemsOption{
		CustomerId: authCustomer.Id,
		CartIds:    []int64{0},
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := TransformCartItemsToReplyForMP(page.List)
	return &types.ListCartItemsPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil
}

func TransformCartItemsToReplyForMP(items []*trade.CartItem) []*types.CartItem {

	itemsReply := []*types.CartItem{}
	for _, item := range items {
		itemReply := TransformCartItemToReplyForMP(item)
		itemsReply = append(itemsReply, itemReply)
	}
	return itemsReply
}
