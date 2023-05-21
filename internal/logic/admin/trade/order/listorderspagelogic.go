package order

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListOrdersPageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListOrdersPageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListOrdersPageLogic {
	return &ListOrdersPageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListOrdersPageLogic) ListOrdersPage(req *types.ListOrdersPageRequest) (resp *types.ListOrdersPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
