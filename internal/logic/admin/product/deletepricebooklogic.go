package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeletePriceBookLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeletePriceBookLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeletePriceBookLogic {
	return &DeletePriceBookLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeletePriceBookLogic) DeletePriceBook(req *types.DeletePriceBookRequest) (resp *types.DeletePriceBookReply, err error) {
	// todo: add your logic here and delete this line

	return
}
