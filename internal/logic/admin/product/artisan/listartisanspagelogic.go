package artisan

import (
	product2 "PowerX/internal/model/product"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListArtisansLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListArtisansPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListArtisansLogic {
	return &ListArtisansLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListArtisansLogic) ListArtisansPage(req *types.ListArtisansPageRequest) (resp *types.ListArtisansPageReply, err error) {
	page, err := l.svcCtx.PowerX.Artisan.FindManyArtisans(l.ctx, &product2.FindArtisanOption{
		OrderBy: req.OrderBy,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})

	if err != nil {
		return nil, err
	}

	list := TransferArtisansToArtisansReply(page.List)
	return &types.ListArtisansPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil

}

func TransferArtisansToArtisansReply(artisans []*product2.Artisan) []*types.Artisan {
	artisansReply := []*types.Artisan{}
	for _, artisan := range artisans {
		artisanReply := TransferArtisanToArtisanReply(artisan)
		artisansReply = append(artisansReply, artisanReply)
	}
	return artisansReply
}
