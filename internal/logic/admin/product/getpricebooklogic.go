package product

import (
	"PowerX/internal/types/errorx"
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
	priceBook, err := l.svcCtx.PowerX.PriceBook.GetPriceBook(l.ctx, req.PriceBook)

	if err != nil {
		return nil, errorx.ErrNotFoundObject
	}

	return &types.GetPriceBookReply{
		PriceBook: &types.PriceBook{
			Id:          priceBook.Id,
			IsStandard:  priceBook.IsStandard,
			Name:        priceBook.Name,
			Description: priceBook.Description,
			CreatedAt:   priceBook.CreatedAt.String(),
		},
	}, nil

	return
}
