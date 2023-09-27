package token

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteTokenProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteTokenProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteTokenProductLogic {
	return &DeleteTokenProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteTokenProductLogic) DeleteTokenProduct(req *types.DeleteProductRequest) (resp *types.DeleteProductReply, err error) {
	// todo: add your logic here and delete this line

	return
}
