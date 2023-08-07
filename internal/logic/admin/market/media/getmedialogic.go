package media

import (
	"PowerX/internal/model/market"
	"PowerX/internal/model/media"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetMediaLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetMediaLogic {
	return &GetMediaLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetMediaLogic) GetMedia(req *types.GetMediaRequest) (resp *types.GetMediaReply, err error) {
	// todo: add your logic here and delete this line

	return
}

func TransformMediaToMediaReply(mdlMedia *market.Media) (mediaReply *types.Media) {

	return &types.Media{
		Id:             mdlMedia.Id,
		Title:          mdlMedia.Title,
		SubTitle:       mdlMedia.SubTitle,
		CoverImageId:   mdlMedia.CoverImageId,
		ResourceUrl:    mdlMedia.ResourceUrl,
		Description:    mdlMedia.Description,
		MediaType:      mdlMedia.MediaType,
		ViewedCount:    mdlMedia.ViewedCount,
		CoverImage:     TransformMediaImageToMediaImageReply(mdlMedia.CoverImage),
		DetailImageIds: media.GetImageIds(mdlMedia.PivotDetailImages),
		DetailImages:   TransformMediaImagesToImagesReply(mdlMedia.PivotDetailImages),
	}
}

func TransformMediaImageToMediaImageReply(resource *media.MediaResource) *types.MediaImage {
	if resource == nil {
		return nil
	}
	return &types.MediaImage{
		Id:            resource.Id,
		BucketName:    resource.BucketName,
		Filename:      resource.Filename,
		Size:          resource.Size,
		IsLocalStored: resource.IsLocalStored,
		Url:           resource.Url,
		ContentType:   resource.ContentType,
		ResourceType:  resource.ResourceType,
	}
}

func TransformMediaImagesToImagesReply(pivots []*media.PivotMediaResourceToObject) (imagesReply []*types.MediaImage) {

	imagesReply = []*types.MediaImage{}
	for _, pivot := range pivots {
		imageReply := TransformMediaImageToMediaImageReply(pivot.MediaResource)
		imagesReply = append(imagesReply, imageReply)
	}
	return imagesReply
}
