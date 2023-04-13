package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchProductLogic {
	return &PatchProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchProductLogic) PatchProduct(req *types.PatchProductRequest) (resp *types.PatchProductReply, err error) {
	// todo: add your logic here and delete this line

	return
}
