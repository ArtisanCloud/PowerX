package artisan

import (
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
	// todo: add your logic here and delete this line

	return
}
