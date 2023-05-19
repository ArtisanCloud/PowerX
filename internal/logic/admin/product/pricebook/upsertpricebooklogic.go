package pricebook

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpsertPriceBookLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpsertPriceBookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpsertPriceBookLogic {
	return &UpsertPriceBookLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpsertPriceBookLogic) UpsertPriceBook(req *types.UpsertPriceBookRequest) (resp *types.UpsertPriceBookReply, err error) {
	// todo: add your logic here and delete this line

	return
}
