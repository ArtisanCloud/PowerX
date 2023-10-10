package artisan

import (
	"PowerX/internal/logic/admin/crm/product/artisan"
	product3 "PowerX/internal/uc/powerx/crm/product"
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
	list := artisan.TransformArtisansToReply(artisans.List)

	return &types.ListArtisansPageReply{
		List:      list,
		PageIndex: artisans.PageIndex,
		PageSize:  artisans.PageSize,
		Total:     artisans.Total,
	}, nil
}
