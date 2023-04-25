package product

import (
	"PowerX/internal/uc/powerx/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListStoresLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListStoresLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListStoresLogic {
	return &ListStoresLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListStoresLogic) ListStores(req *types.GetStoreListRequest) (resp *types.GetStoreListReply, err error) {
	stores, err := l.svcCtx.PowerX.Store.FindAllShops(l.ctx, &product.FindManyStoresOption{})

	if err != nil {
		return nil, err
	}

	list := make([]types.Store, 0, len(stores))
	for _, item := range stores {
		list = append(list, types.Store{
			Id:            item.Id,
			Name:          item.Name,
			EmployeeId:    item.EmployeeId,
			ContactNumber: item.ContactNumber,
			CoverURL:      item.CoverURL,
			Address:       item.Address,
			Longitude:     item.Longitude,
			Latitude:      item.Latitude,
			CreatedAt:     item.CreatedAt.String(),
		})
	}

	return &types.GetStoreListReply{
		List: list,
	}, nil

	return

	return
}
