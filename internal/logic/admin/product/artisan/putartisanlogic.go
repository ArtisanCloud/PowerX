package artisan

import (
	"PowerX/internal/model/media"
	"PowerX/internal/uc/powerx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutArtisanLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutArtisanLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutArtisanLogic {
	return &PutArtisanLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutArtisanLogic) PutArtisan(req *types.PutArtisanRequest) (resp *types.PutArtisanReply, err error) {
	mdlArtisan := TransformArtisanRequestToArtisan(&(req.Artisan))
	mdlArtisan.Id = req.ArtisanId
	if len(req.DetailImageIds) > 0 {
		mediaResources, err := l.svcCtx.PowerX.MediaResource.FindAllMediaResources(l.ctx, &powerx.FindManyMediaResourcesOption{
			Ids: req.DetailImageIds,
		})
		if err != nil {
			return nil, err
		}
		mdlArtisan.PivotDetailImages, err = (&media.PivotMediaResourceToObject{}).MakeMorphPivotsFromObjectToMediaResources(mdlArtisan, mediaResources, media.MediaUsageDetail)
	}

	// 更新产品对象
	mdlArtisan, err = l.svcCtx.PowerX.Artisan.UpsertArtisan(l.ctx, mdlArtisan)
	if err != nil {
		return nil, err
	}

	return &types.PutArtisanReply{
		Artisan: TransformArtisanToArtisanReply(mdlArtisan),
	}, nil
}
