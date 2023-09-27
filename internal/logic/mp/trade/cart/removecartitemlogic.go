package cart

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/model/trade"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/crm/customerdomain"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type RemoveCartItemLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRemoveCartItemLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveCartItemLogic {
	return &RemoveCartItemLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *RemoveCartItemLogic) RemoveCartItem(req *types.RemoveCartItemRequest) (resp *types.RemoveCartItemReply, err error) {

	vAuthCustomer := l.ctx.Value(customerdomain.AuthCustomerKey)
	authCustomer := vAuthCustomer.(*customerdomain2.Customer)

	mdlCartItems := []*trade.CartItem{
		{
			PowerModel: &powermodel.PowerModel{
				Id: req.ItemId,
			},
			CustomerId: authCustomer.Id,
		},
	}

	err = l.svcCtx.PowerX.Cart.RemoveItemsFromCart(l.ctx, mdlCartItems)

	if err != nil {
		return nil, errorx.WithCause(errorx.ErrDeleteObjectNotFound, err.Error())
	}

	return &types.RemoveCartItemReply{
		ItemId: req.ItemId,
	}, nil

}
