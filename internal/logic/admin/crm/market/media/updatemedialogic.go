package media

import (
	"PowerX/internal/model/media"
	"PowerX/internal/uc/powerx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateMediaLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateMediaLogic {
	return &UpdateMediaLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateMediaLogic) UpdateMedia(req *types.UpdateMediaRequest) (resp *types.UpdateMediaReply, err error) {
	mdlMedia := TransformRequestToMedia(&(req.Media))
	mdlMedia.Id = req.MediaId

	if len(req.DetailImageIds) > 0 {
		mediaResources, err := l.svcCtx.PowerX.MediaResource.FindAllMediaResources(l.ctx, &powerx.FindManyMediaResourcesOption{
			Ids: req.DetailImageIds,
		})
		if err != nil {
			return nil, err
		}
		mdlMedia.PivotDetailImages, err = (&media.PivotMediaResourceToObject{}).MakeMorphPivotsFromObjectToMediaResources(mdlMedia, mediaResources, media.MediaUsageDetail)
	}

	// 更新产品对象
	mdlMedia, err = l.svcCtx.PowerX.Media.UpsertMedia(l.ctx, mdlMedia)
	if err != nil {
		return nil, err
	}

	return &types.UpdateMediaReply{
		MediaId: mdlMedia.Id,
	}, nil
}
