package store

import (
	"PowerX/internal/model/crm/market"
	"PowerX/internal/model/media"
	"PowerX/internal/uc/powerx"
	"PowerX/pkg/datetime/carbonx"
	"context"
	"github.com/golang-module/carbon/v2"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateStoreLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateStoreLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateStoreLogic {
	return &CreateStoreLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateStoreLogic) CreateStore(req *types.CreateStoreRequest) (resp *types.CreateStoreReply, err error) {
	mdlStore := TransformRequestToStore(&req.Store)

	if len(req.DetailImageIds) > 0 {
		mediaResources, err := l.svcCtx.PowerX.MediaResource.FindAllMediaResources(l.ctx, &powerx.FindManyMediaResourcesOption{
			Ids: req.DetailImageIds,
		})
		if err != nil {
			return nil, err
		}
		mdlStore.PivotDetailImages, err = (&media.PivotMediaResourceToObject{}).MakeMorphPivotsFromObjectToMediaResources(mdlStore, mediaResources, media.MediaUsageDetail)
	}

	err = l.svcCtx.PowerX.Store.CreateStore(l.ctx, mdlStore)

	return &types.CreateStoreReply{
		StoreId: mdlStore.Id,
	}, nil
}

func TransformRequestToStore(storeRequest *types.Store) (mdlStore *market.Store) {

	startWork := carbon.ParseByFormat(storeRequest.StartWork, carbonx.TimeFormat)
	endWork := carbon.ParseByFormat(storeRequest.EndWork, carbonx.TimeFormat)
	return &market.Store{
		StoreEmployeeId: storeRequest.StoreEmployeeId,
		Name:            storeRequest.Name,
		ContactNumber:   storeRequest.ContactNumber,
		CoverImageId:    storeRequest.CoverImageId,
		Email:           storeRequest.Email,
		Address:         storeRequest.Address,
		Description:     storeRequest.Description,
		Longitude:       storeRequest.Longitude,
		Latitude:        storeRequest.Latitude,
		StartWork:       startWork.ToStdTime(),
		EndWork:         endWork.ToStdTime(),
	}
}
