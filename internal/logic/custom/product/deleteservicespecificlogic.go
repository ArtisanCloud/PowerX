package product

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteServiceSpecificLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteServiceSpecificLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteServiceSpecificLogic {
	return &DeleteServiceSpecificLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteServiceSpecificLogic) DeleteServiceSpecific(req *types.DeleteServiceSpecificRequest) (resp *types.DeleteServiceSpecificReply, err error) {
	// todo: add your logic here and delete this line

	return
}
