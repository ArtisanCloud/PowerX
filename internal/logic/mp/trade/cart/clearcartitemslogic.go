package cart

import (
	customerdomain2 "PowerX/internal/model/customerdomain"
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/customerdomain"
	trade2 "PowerX/internal/uc/powerx/trade"
	"context"
	"fmt"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ClearCartItemsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewClearCartItemsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ClearCartItemsLogic {
	return &ClearCartItemsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ClearCartItemsLogic) ClearCartItems(req *types.ClearCartItemsRequest) (resp *types.ClearCartItemsReply, err error) {
	vAuthUser := l.ctx.Value(customerdomain.AuthCustomerKey)
	authUser := vAuthUser.(*customerdomain2.Customer)

	page, err := l.svcCtx.PowerX.Cart.FindManyCartItems(l.ctx, &trade2.FindManyCartsOption{
		CustomerId: authUser.Id,
		CartId:     0,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: 0,
			PageSize:  powermodel.MaxPageSize,
		},
	})

	if err != nil {
		return nil, errorx.WithCause(errorx.ErrDeleteObjectNotFound, err.Error())
	}

	err = l.svcCtx.PowerX.Cart.RemoveItemsFromCart(l.ctx, page.List)

	if err != nil {
		return nil, errorx.WithCause(errorx.ErrDeleteObject, err.Error())
	}

	return &types.ClearCartItemsReply{
		Message: fmt.Sprintf("清空记录共计%d项", page.Total),
	}, nil
}
