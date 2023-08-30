package tag

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListTagTreeLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListTagTreeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListTagTreeLogic {
	return &ListTagTreeLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListTagTreeLogic) ListTagTree(req *types.ListTagTreeRequest) (resp *types.ListTagTreeReply, err error) {
	// todo: add your logic here and delete this line

	return
}
