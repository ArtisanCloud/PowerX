package sku

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutSKULogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutSKULogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutSKULogic {
	return &PutSKULogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutSKULogic) PutSKU(req *types.PutSKURequest) (resp *types.PutSKUReply, err error) {
	// todo: add your logic here and delete this line

	return
}
