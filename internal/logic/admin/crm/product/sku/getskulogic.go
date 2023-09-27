package sku

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetSKULogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetSKULogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetSKULogic {
	return &GetSKULogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetSKULogic) GetSKU(req *types.GetSKURequest) (resp *types.GetSKUReply, err error) {
	// todo: add your logic here and delete this line

	return
}
