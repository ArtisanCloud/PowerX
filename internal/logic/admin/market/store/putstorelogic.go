package store

import (
	"PowerX/internal/model/media"
	"PowerX/internal/uc/powerx"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutStoreLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutStoreLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutStoreLogic {
	return &PutStoreLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutStoreLogic) PutStore(req *types.PutStoreRequest) (resp *types.PutStoreReply, err error) {

	mdlStore := TransformRequestToStore(&(req.Store))
	mdlStore.Id = req.StoreId
	if len(req.DetailImageIds) > 0 {
		mediaResources, err := l.svcCtx.PowerX.MediaResource.FindAllMediaResources(l.ctx, &powerx.FindManyMediaResourcesOption{
			Ids: req.DetailImageIds,
		})
		if err != nil {
			return nil, err
		}
		mdlStore.PivotDetailImages, err = (&media.PivotMediaResourceToObject{}).MakeMorphPivotsFromObjectToMediaResources(mdlStore, mediaResources, media.MediaUsageDetail)
	}

	// 更新产品对象
	mdlStore, err = l.svcCtx.PowerX.Store.UpsertStore(l.ctx, mdlStore)
	if err != nil {
		return nil, err
	}

	return &types.PutStoreReply{
		Store: TransformStoreToReply(mdlStore),
	}, nil

}
