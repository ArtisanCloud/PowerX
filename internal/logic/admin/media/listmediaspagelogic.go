package media

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListMediasPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListMediasPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMediasPageLogic {
	return &ListMediasPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListMediasPageLogic) ListMediasPage(req *types.ListMediasPageRequest) (resp *types.ListMediasPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
