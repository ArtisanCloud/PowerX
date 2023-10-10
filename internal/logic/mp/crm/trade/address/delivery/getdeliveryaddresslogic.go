package delivery

import (
	"context"

	"PowerX/internal/svc"
	"PowerX/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetDeliveryAddressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetDeliveryAddressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetDeliveryAddressLogic {
	return &GetDeliveryAddressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetDeliveryAddressLogic) GetDeliveryAddress(req *types.GetDeliveryAddressRequest) (resp *types.GetDeliveryAddressReply, err error) {
	// todo: add your logic here and delete this line

	return
}
