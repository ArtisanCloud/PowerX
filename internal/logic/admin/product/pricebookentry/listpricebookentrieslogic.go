package pricebookentry

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListPriceBookEntriesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListPriceBookEntriesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPriceBookEntriesLogic {
	return &ListPriceBookEntriesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListPriceBookEntriesLogic) ListPriceBookEntries(req *types.ListPriceBookEntriesPageRequest) (resp *types.ListPriceBookEntriesPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
