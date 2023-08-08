package media

import (
	"PowerX/internal/logic/admin/mediaresource"
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

func TransformMediaToReply(mdlMedia *market.Media) (mediaReply *types.Media) {

	return &types.Media{
		Id:             mdlMedia.Id,
		Title:          mdlMedia.Title,
		SubTitle:       mdlMedia.SubTitle,
		CoverImageId:   mdlMedia.CoverImageId,
		ResourceUrl:    mdlMedia.ResourceUrl,
		Description:    mdlMedia.Description,
		MediaType:      mdlMedia.MediaType,
		ViewedCount:    mdlMedia.ViewedCount,
		CoverImage:     mediaresource.TransformMediaResourceToReply(mdlMedia.CoverImage),
		DetailImageIds: media.GetImageIds(mdlMedia.PivotDetailImages),
		DetailImages:   mediaresource.TransformMediaResourcesToReply(mdlMedia.PivotDetailImages),
	}
}
