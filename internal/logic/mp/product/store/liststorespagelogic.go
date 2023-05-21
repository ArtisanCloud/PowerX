package store

import (
	product2 "PowerX/internal/logic/admin/product"
	"PowerX/internal/uc/powerx/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListStoresPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListStoresPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListStoresPageLogic {
	return &ListStoresPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListStoresPageLogic) ListStoresPage(req *types.ListStoresPageRequest) (resp *types.ListStoresPageReply, err error) {
	stores, err := l.svcCtx.PowerX.Store.FindManyStores(l.ctx, &product.FindManyStoresOption{
		LikeName: req.LikeName,
		OrderBy:  req.OrderBy,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})

	if err != nil {
		return nil, err
	}
	list := product2.TransferStoresToStoresReply(stores.List)

	return &types.ListStoresPageReply{
		List:      list,
		PageIndex: stores.PageIndex,
		PageSize:  stores.PageSize,
		Total:     stores.Total,
	}, nil
}
