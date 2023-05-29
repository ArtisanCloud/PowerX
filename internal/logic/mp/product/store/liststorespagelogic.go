package store

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListStoresPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListStoresPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListStoresPageLogic {
	return &ListStoresPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListStoresPageLogic) ListStoresPage(req *types.ListStoresPageRequest) (resp *types.ListStoresPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
