package artisan

import (
	"PowerX/internal/model/powermodel"
	"PowerX/internal/types/errorx"
	"PowerX/internal/uc/powerx/crm/market"
	product2 "PowerX/internal/uc/powerx/crm/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type BindArtisanToStoreLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBindArtisanToStoreLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BindArtisanToStoreLogic {
	return &BindArtisanToStoreLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *BindArtisanToStoreLogic) BindArtisanToStore(req *types.BindArtisansToStoresRequest) (resp *types.BindArtisansToStoresReply, err error) {

	if len(req.StoreId) <= 0 || len(req.ArtisanIds) <= 0 {
		return nil, errorx.WithCause(errorx.ErrBadRequest, "上传参数有误")
	}

	// 根据ArtisanIds，查找元匠对象
	resArtisans, err := l.svcCtx.PowerX.Artisan.FindManyArtisans(l.ctx, &product2.FindManyArtisanOption{
		Ids: req.ArtisanIds,
		PageEmbedOption: types.PageEmbedOption{
			PageSize: powermodel.MaxPageSize,
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resArtisans.List) <= 0 {
		return nil, errorx.WithCause(errorx.ErrNotFoundObject, "未找到元匠对象")
	}

	// 根据StoreIds，查找门店对象
	resStores, err := l.svcCtx.PowerX.Store.FindManyStores(l.ctx, &market.FindManyStoresOption{
		Ids: req.StoreId,
		PageEmbedOption: types.PageEmbedOption{
			PageSize: powermodel.MaxPageSize,
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resStores.List) <= 0 {
		return nil, errorx.WithCause(errorx.ErrNotFoundObject, "未找到门店对象")
	}

	// 将元匠批量绑定到门店

	err = l.svcCtx.PowerX.Artisan.BindArtisansToStores(l.ctx, resArtisans.List, resStores.List)
	if err != nil {
		return nil, err
	}

	return &types.BindArtisansToStoresReply{}, nil

}
