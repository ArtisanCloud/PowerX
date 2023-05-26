package mediaresource

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteMediaResourceLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteMediaResourceLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteMediaResourceLogic {
	return &DeleteMediaResourceLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteMediaResourceLogic) DeleteMediaResource(req *types.DeleteMediaResourceRequest) (resp *types.DeleteMediaResourceReply, err error) {
	// todo: add your logic here and delete this line

	return
}
