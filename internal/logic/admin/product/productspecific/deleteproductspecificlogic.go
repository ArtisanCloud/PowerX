package productspecific

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteProductSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteProductSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteProductSpecificLogic {
	return &DeleteProductSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteProductSpecificLogic) DeleteProductSpecific(req *types.DeleteProductSpecificRequest) (resp *types.DeleteProductSpecificReply, err error) {
	// todo: add your logic here and delete this line

	return
}
