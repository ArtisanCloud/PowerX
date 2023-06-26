package sku

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchSKULogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchSKULogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchSKULogic {
	return &PatchSKULogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchSKULogic) PatchSKU(req *types.PatchSKURequest) (resp *types.PatchSKUReply, err error) {
	// todo: add your logic here and delete this line

	return
}
