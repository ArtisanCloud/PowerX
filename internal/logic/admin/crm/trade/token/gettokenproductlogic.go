package token

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetTokenProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetTokenProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetTokenProductLogic {
	return &GetTokenProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetTokenProductLogic) GetTokenProduct(req *types.GetProductRequest) (resp *types.GetProductReply, err error) {
	// todo: add your logic here and delete this line

	return
}
