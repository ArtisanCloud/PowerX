package pricebook

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetPriceBookLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetPriceBookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetPriceBookLogic {
	return &GetPriceBookLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetPriceBookLogic) GetPriceBook(req *types.GetPriceBookRequest) (resp *types.GetPriceBookReply, err error) {
	// todo: add your logic here and delete this line

	return
}
