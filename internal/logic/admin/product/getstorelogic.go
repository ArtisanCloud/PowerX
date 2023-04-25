package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetStoreLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetStoreLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetStoreLogic {
	return &GetStoreLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetStoreLogic) GetStore(req *types.GetStoreRequest) (resp *types.GetStoreReply, err error) {
	// todo: add your logic here and delete this line

	return
}
