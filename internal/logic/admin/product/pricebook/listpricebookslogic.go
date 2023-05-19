package pricebook

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListPriceBooksLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListPriceBooksLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListPriceBooksLogic {
	return &ListPriceBooksLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListPriceBooksLogic) ListPriceBooks(req *types.ListPriceBooksPageRequest) (resp *types.ListPriceBooksPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
