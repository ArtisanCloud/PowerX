package trade

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UpdateCartItemQuantityLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateCartItemQuantityLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateCartItemQuantityLogic {
	return &UpdateCartItemQuantityLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UpdateCartItemQuantityLogic) UpdateCartItemQuantity(req *types.UpdateCartItemQuantityRequest) (resp *types.UpdateCartItemQuantityReply, err error) {
	// todo: add your logic here and delete this line

	return
}
