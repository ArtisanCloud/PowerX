package order

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchOrderLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchOrderLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchOrderLogic {
	return &PatchOrderLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchOrderLogic) PatchOrder(req *types.PatchOrderRequest) (resp *types.PatchOrderReply, err error) {
	// todo: add your logic here and delete this line

	return
}
