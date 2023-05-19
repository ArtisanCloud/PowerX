package store

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PatchStoreLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPatchStoreLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PatchStoreLogic {
	return &PatchStoreLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PatchStoreLogic) PatchStore(req *types.PatchStoreRequest) (resp *types.PatchStoreReply, err error) {
	// todo: add your logic here and delete this line

	return
}
