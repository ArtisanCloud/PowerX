package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListMediasLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListMediasLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMediasLogic {
	return &ListMediasLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListMediasLogic) ListMedias(req *types.ListMediasRequest) (resp *types.ListMediasReply, err error) {
	// todo: add your logic here and delete this line

	return
}
