package sku

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListSKUPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListSKUPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListSKUPageLogic {
	return &ListSKUPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListSKUPageLogic) ListSKUPage(req *types.ListSKUPageRequest) (resp *types.ListSKUPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
