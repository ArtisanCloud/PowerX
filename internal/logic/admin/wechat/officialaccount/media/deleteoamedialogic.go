package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteOAMediaLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteOAMediaLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteOAMediaLogic {
	return &DeleteOAMediaLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteOAMediaLogic) DeleteOAMedia(req *types.DeleteOAMediaRequest) (resp *types.DeleteOAMediaReply, err error) {
	// todo: add your logic here and delete this line

	return
}
