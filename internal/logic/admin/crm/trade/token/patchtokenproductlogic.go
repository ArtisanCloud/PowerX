package token

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchTokenProductLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchTokenProductLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchTokenProductLogic {
	return &PatchTokenProductLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchTokenProductLogic) PatchTokenProduct(req *types.PatchProductRequest) (resp *types.PatchProductReply, err error) {
	// todo: add your logic here and delete this line

	return
}
