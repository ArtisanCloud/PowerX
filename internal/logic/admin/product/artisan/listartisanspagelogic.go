package artisan

import (
	"PowerX/internal/model/media"
	"PowerX/internal/model/product"
	product3 "PowerX/internal/uc/powerx/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListArtisansPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListArtisansPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListArtisansPageLogic {
	return &ListArtisansPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListArtisansPageLogic) ListArtisansPage(req *types.ListArtisansPageRequest) (resp *types.ListArtisansPageReply, err error) {
	artisans, err := l.svcCtx.PowerX.Artisan.FindManyArtisans(l.ctx, &product3.FindManyArtisanOption{
		LikeName: req.LikeName,
		OrderBy:  req.OrderBy,
		StoreIds: req.StoreIds,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})

	if err != nil {
		return nil, err
	}
	list := TransformArtisansToReply(artisans.List)

	return &types.ListArtisansPageReply{
		List:      list,
		PageIndex: artisans.PageIndex,
		PageSize:  artisans.PageSize,
		Total:     artisans.Total,
	}, nil
}

func TransformArtisansToReply(artisans []*product.Artisan) []*types.Artisan {
	artisansReply := []*types.Artisan{}
	for _, artisan := range artisans {
		artisanReply := TransformArtisanToReply(artisan)
		artisansReply = append(artisansReply, artisanReply)
	}
	return artisansReply
}

func TransformArtisanToReply(artisan *product.Artisan) *types.Artisan {
	return &types.Artisan{
		Id:             artisan.Id,
		EmployeeId:     artisan.EmployeeId,
		Name:           artisan.Name,
		Level:          artisan.Level,
		Gender:         artisan.Gender,
		Birthday:       artisan.Birthday.String(),
		PhoneNumber:    artisan.PhoneNumber,
		WorkNo:         artisan.WorkNo,
		Email:          artisan.Email,
		Experience:     artisan.Experience,
		Specialty:      artisan.Specialty,
		Certificate:    artisan.Certificate,
		Address:        artisan.Address,
		CreatedAt:      artisan.CreatedAt.String(),
		CoverImageId:   artisan.CoverImageId,
		CoverImage:     TransformArtisanImageToReply(artisan.CoverImage),
		DetailImageIds: media.GetImageIds(artisan.PivotDetailImages),
		DetailImages:   TransformArtisanImagesToReply(artisan.PivotDetailImages),
		StoreIds:       product.GetStoreIds(artisan.PivotStoreToArtisans),
	}
}

func TransformArtisanImageToReply(resource *media.MediaResource) *types.MediaResource {
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

func TransformArtisanImagesToReply(pivots []*media.PivotMediaResourceToObject) (imagesReply []*types.MediaResource) {

	imagesReply = []*types.MediaResource{}
	for _, pivot := range pivots {
		imageReply := TransformArtisanImageToReply(pivot.MediaResource)
		imagesReply = append(imagesReply, imageReply)
	}
	return imagesReply
}
