package delivery

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteDeliveryAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteDeliveryAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteDeliveryAddressLogic {
	return &DeleteDeliveryAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *DeleteDeliveryAddressLogic) DeleteDeliveryAddress(req *types.DeleteDeliveryAddressRequest) (resp *types.DeleteDeliveryAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
