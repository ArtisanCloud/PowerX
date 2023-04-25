package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteStoreLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteStoreLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteStoreLogic {
	return &DeleteStoreLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteStoreLogic) DeleteStore(req *types.DeleteStoreRequest) (resp *types.DeleteStoreReply, err error) {
	// todo: add your logic here and delete this line

	return
}
