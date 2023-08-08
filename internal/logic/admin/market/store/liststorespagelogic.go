package store

import (
	product2 "PowerX/internal/model/market"
	"PowerX/internal/model/media"
	"PowerX/internal/uc/powerx/market"
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

func NewListStoresPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListStoresLogic {
	return &ListStoresLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListStoresLogic) ListStoresPage(req *types.ListStoresPageRequest) (resp *types.ListStoresPageReply, err error) {
	stores, err := l.svcCtx.PowerX.Store.FindManyStores(l.ctx, &market.FindManyStoresOption{
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
	list := TransformStoresToReply(stores.List)

	return &types.ListStoresPageReply{
		List:      list,
		PageIndex: stores.PageIndex,
		PageSize:  stores.PageSize,
		Total:     stores.Total,
	}, nil

}

func TransformStoresToReply(stores []*product2.Store) []*types.Store {
	storesReply := []*types.Store{}
	for _, store := range stores {
		storeReply := TransformStoreToStoreReply(store)
		storesReply = append(storesReply, storeReply)
	}
	return storesReply
}

func TransformStoreToStoreReply(store *product2.Store) *types.Store {
	return &types.Store{
		Id:              store.Id,
		Name:            store.Name,
		StoreEmployeeId: store.StoreEmployeeId,
		ContactNumber:   store.ContactNumber,
		Address:         store.Address,
		Description:     store.Description,
		Longitude:       store.Longitude,
		Latitude:        store.Latitude,
		StartWork:       store.StartWork.String(),
		EndWork:         store.EndWork.String(),
		CreatedAt:       store.CreatedAt.String(),
		CoverImageId:    store.CoverImageId,
		CoverImage:      TransformStoreImageToReply(store.CoverImage),
		DetailImageIds:  media.GetImageIds(store.PivotDetailImages),
		DetailImages:    TransformStoreImagesToImagesReply(store.PivotDetailImages),
		Artisans:        TransformArtisansToShopArtisans(store.Artisans),
	}
}

func TransformStoreImageToReply(resource *media.MediaResource) *types.MediaResource {
	if resource == nil {
		return nil
	}
	return &types.MediaResource{
		Id:            resource.Id,
		CustomerId:    resource.CustomerId,
		BucketName:    resource.BucketName,
		Filename:      resource.Filename,
		Size:          resource.Size,
		IsLocalStored: resource.IsLocalStored,
		Url:           resource.Url,
		ContentType:   resource.ContentType,
		ResourceType:  resource.ResourceType,
	}
}

func TransformStoreImagesToImagesReply(pivots []*media.PivotMediaResourceToObject) (imagesReply []*types.MediaResource) {

	imagesReply = []*types.MediaResource{}
	for _, pivot := range pivots {
		imageReply := TransformStoreImageToReply(pivot.MediaResource)
		imagesReply = append(imagesReply, imageReply)
	}
	return imagesReply
}
