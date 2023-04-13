package store

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type CreateStoreLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateStoreLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateStoreLogic {
	return &CreateStoreLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *CreateStoreLogic) CreateStore(req *types.CreateStoreRequest) (resp *types.CreateStoreReply, err error) {
	// todo: add your logic here and delete this line

	return
}
