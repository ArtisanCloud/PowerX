package product

import (
	product2 "PowerX/internal/model/product"
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
	list := TransferStoresToStoresReply(stores)

	return &types.GetStoreListReply{
		List: list,
	}, nil

}

func TransferStoresToStoresReply(stores []*product2.Store) []*types.Store {
	storesReply := []*types.Store{}
	for _, store := range stores {
		storeReply := TransferStoreToStoreReply(store)
		storesReply = append(storesReply, storeReply)
	}
	return storesReply
}

func TransferStoreToStoreReply(store *product2.Store) *types.Store {
	return &types.Store{
		Id:            store.Id,
		Name:          store.Name,
		EmployeeId:    store.EmployeeId,
		ContactNumber: store.ContactNumber,
		CoverURL:      store.CoverURL,
		Address:       store.Address,
		Longitude:     store.Longitude,
		Latitude:      store.Latitude,
		CreatedAt:     store.CreatedAt.String(),
		Artisans:      TransferArtisansToStoreArtisans(store.Artisans),
	}
}
