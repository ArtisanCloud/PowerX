package media

import (
	"PowerX/internal/logic/admin/market/media"
	"PowerX/internal/uc/powerx/market"
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListMediasPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListMediasPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMediasPageLogic {
	return &ListMediasPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListMediasPageLogic) ListMediasPage(req *types.ListMediasPageRequest) (resp *types.ListMediasPageReply, err error) {
	page, err := l.svcCtx.PowerX.Media.FindManyMedias(l.ctx, &market.FindManyMediasOption{
		Types: req.MediaTypes,
		PageEmbedOption: types.PageEmbedOption{
			PageIndex: req.PageIndex,
			PageSize:  req.PageSize,
		},
	})
	if err != nil {
		return nil, err
	}

	// list
	list := media.TransformMediasToReply(page.List)
	return &types.ListMediasPageReply{
		List:      list,
		PageIndex: page.PageIndex,
		PageSize:  page.PageSize,
		Total:     page.Total,
	}, nil
}
