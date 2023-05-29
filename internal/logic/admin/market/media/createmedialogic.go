package media

import (
	"PowerX/internal/model/market"
	"PowerX/internal/model/media"
	"PowerX/internal/uc/powerx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateMediaLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateMediaLogic {
	return &CreateMediaLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateMediaLogic) CreateMedia(req *types.CreateMediaRequest) (resp *types.CreateMediaReply, err error) {
	mdlMedia := TransformMediaRequestToMedia(&req.Media)

	if len(req.DetailImageIds) > 0 {
		mediaResources, err := l.svcCtx.PowerX.MediaResource.FindAllMediaResources(l.ctx, &powerx.FindManyMediaResourcesOption{
			Ids: req.DetailImageIds,
		})
		if err != nil {
			return nil, err
		}
		mdlMedia.PivotDetailImages, err = (&media.PivotMediaResourceToObject{}).MakeMorphPivotsFromObjectToMediaResources(mdlMedia, mediaResources, media.MediaUsageDetail)
	}

	l.svcCtx.PowerX.Media.CreateMedia(l.ctx, mdlMedia)

	return &types.CreateMediaReply{
		MediaId: mdlMedia.Id,
	}, nil
}

func TransformMediaRequestToMedia(mediaRequest *types.Media) (mdlMedia *market.Media) {

	return &market.Media{
		Title:        mediaRequest.Title,
		SubTitle:     mediaRequest.SubTitle,
		CoverImageId: mediaRequest.CoverImageId,
		ResourceUrl:  mediaRequest.ResourceUrl,
		Description:  mediaRequest.Description,
		MediaType:    market.MediaType(mediaRequest.MediaType),
	}
}
