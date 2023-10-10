package cart

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListCartItemsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListCartItemsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListCartItemsLogic {
	return &ListCartItemsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListCartItemsLogic) ListCartItems(req *types.ListCartItemsPageRequest) (resp *types.ListCartItemsPageReply, err error) {
	// todo: add your logic here and delete this line

	return
}
