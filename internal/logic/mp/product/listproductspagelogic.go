package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListProductsPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListProductsPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListProductsPageLogic {
	return &ListProductsPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListProductsPageLogic) ListProductsPage(req *types.ListProductsPageRequest) (resp *types.ListProductsPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
