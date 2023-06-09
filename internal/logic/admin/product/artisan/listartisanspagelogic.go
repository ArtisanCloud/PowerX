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
		StoreId:  req.StoreId,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})

	if err != nil {
		return nil, err
	}
	list := TransformArtisansToArtisansReply(artisans.List)

	return &types.ListArtisansPageReply{
		List:      list,
		PageIndex: artisans.PageIndex,
		PageSize:  artisans.PageSize,
		Total:     artisans.Total,
	}, nil
}

func TransformArtisansToArtisansReply(artisans []*product.Artisan) []*types.Artisan {
	artisansReply := []*types.Artisan{}
	for _, artisan := range artisans {
		artisanReply := TransformArtisanToArtisanReply(artisan)
		artisansReply = append(artisansReply, artisanReply)
	}
	return artisansReply
}

func TransformArtisanToArtisanReply(artisan *product.Artisan) *types.Artisan {
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
		CoverImage:     TransformArtisanImageToArtisanImageReply(artisan.CoverImage),
		DetailImageIds: media.GetImageIds(artisan.PivotDetailImages),
		DetailImages:   TransformArtisanImagesToImagesReply(artisan.PivotDetailImages),
	}
}

func TransformArtisanImageToArtisanImageReply(resource *media.MediaResource) *types.ArtisanImage {
	if resource == nil {
		return nil
	}
	return &types.ArtisanImage{
		Id:           resource.Id,
		BucketName:   resource.BucketName,
		Filename:     resource.Filename,
		Size:         resource.Size,
		Url:          resource.Url,
		ContentType:  resource.ContentType,
		ResourceType: resource.ResourceType,
	}
}

func TransformArtisanImagesToImagesReply(pivots []*media.PivotMediaResourceToObject) (imagesReply []*types.ArtisanImage) {

	imagesReply = []*types.ArtisanImage{}
	for _, pivot := range pivots {
		imageReply := TransformArtisanImageToArtisanImageReply(pivot.MediaResource)
		imagesReply = append(imagesReply, imageReply)
	}
	return imagesReply
}
