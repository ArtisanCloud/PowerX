package shipping

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteShippingAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteShippingAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteShippingAddressLogic {
	return &DeleteShippingAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteShippingAddressLogic) DeleteShippingAddress(req *types.DeleteShippingAddressRequest) (resp *types.DeleteShippingAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
