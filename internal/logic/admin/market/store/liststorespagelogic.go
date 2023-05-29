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
	list := TransformStoresToStoresReply(stores.List)

	return &types.ListStoresPageReply{
		List:      list,
		PageIndex: stores.PageIndex,
		PageSize:  stores.PageSize,
		Total:     stores.Total,
	}, nil

}

func TransformStoresToStoresReply(stores []*product2.Store) []*types.Store {
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
		CoverImage:      TransformStoreImageToStoreImageReply(store.CoverImage),
		DetailImageIds:  media.GetImageIds(store.PivotDetailImages),
		DetailImages:    TransformStoreImagesToImagesReply(store.PivotDetailImages),
		Artisans:        TransformArtisansToShopArtisans(store.Artisans),
	}
}

func TransformStoreImageToStoreImageReply(resource *media.MediaResource) *types.StoreImage {
	if resource == nil {
		return nil
	}
	return &types.StoreImage{
		Id:           resource.Id,
		BucketName:   resource.BucketName,
		Filename:     resource.Filename,
		Size:         resource.Size,
		Url:          resource.Url,
		ContentType:  resource.ContentType,
		ResourceType: resource.ResourceType,
	}
}

func TransformStoreImagesToImagesReply(pivots []*media.PivotMediaResourceToObject) (imagesReply []*types.StoreImage) {

	imagesReply = []*types.StoreImage{}
	for _, pivot := range pivots {
		imageReply := TransformStoreImageToStoreImageReply(pivot.MediaResource)
		imagesReply = append(imagesReply, imageReply)
	}
	return imagesReply
}
