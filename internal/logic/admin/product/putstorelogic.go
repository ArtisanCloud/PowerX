package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type PutStoreLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPutStoreLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PutStoreLogic {
	return &PutStoreLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *PutStoreLogic) PutStore(req *types.PutStoreRequest) (resp *types.PutStoreReply, err error) {
	// todo: add your logic here and delete this line

	return
}
