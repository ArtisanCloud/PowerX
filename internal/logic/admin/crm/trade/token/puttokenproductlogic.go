package token

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutTokenProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutTokenProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutTokenProductLogic {
	return &PutTokenProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutTokenProductLogic) PutTokenProduct(req *types.PutProductRequest) (resp *types.PutProductReply, err error) {
	// todo: add your logic here and delete this line

	return
}
