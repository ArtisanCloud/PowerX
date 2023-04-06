package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteMediaLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteMediaLogic {
	return &DeleteMediaLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteMediaLogic) DeleteMedia(req *types.DeleteMediaRequest) (resp *types.DeleteMediaReply, err error) {
	// todo: add your logic here and delete this line

	return
}
